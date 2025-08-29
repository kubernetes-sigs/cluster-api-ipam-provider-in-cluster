/*
Copyright 2023 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package poolutil implements utility functions to manage a pool of IP addresses.
package poolutil

import (
	"context"
	"errors"
	"math"
	"math/big"
	"net/netip"
	"strings"

	"go4.org/netipx"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ipamv1 "sigs.k8s.io/cluster-api/api/ipam/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/cluster-api-ipam-provider-in-cluster/api/v1alpha2"
	"sigs.k8s.io/cluster-api-ipam-provider-in-cluster/internal/index"
)

// AddressesOutOfRangeIPSet returns an IPSet of the inUseAddresses IPs that are
// not in the poolIPSet.
func AddressesOutOfRangeIPSet(inUseAddresses []ipamv1.IPAddress, poolIPSet *netipx.IPSet) (*netipx.IPSet, error) {
	outOfRangeBuilder := &netipx.IPSetBuilder{}
	for _, address := range inUseAddresses {
		ip, err := netip.ParseAddr(address.Spec.Address)
		if err != nil {
			// if an address we fetch for the pool is unparsable then it isn't in the pool ranges
			continue
		}
		outOfRangeBuilder.Add(ip)
	}
	outOfRangeBuilder.RemoveSet(poolIPSet)
	return outOfRangeBuilder.IPSet()
}

// ListAddressesInUse fetches all IPAddresses belonging to the specified pool.
// Note: requires `index.ipAddressByCombinedPoolRef` to be set up.
func ListAddressesInUse(ctx context.Context, c client.Reader, namespace string, poolRef ipamv1.IPPoolReference) ([]ipamv1.IPAddress, error) {
	addresses := &ipamv1.IPAddressList{}
	err := c.List(ctx, addresses,
		client.MatchingFields{
			index.IPAddressPoolRefCombinedField: index.IPPoolRefValue(poolRef),
		},
		client.InNamespace(namespace),
	)
	addr := []ipamv1.IPAddress{}
	for _, a := range addresses.Items {
		gv, _ := schema.ParseGroupVersion(a.APIVersion)
		if gv.Group != "ipam.cluster.x-k8s.io" {
			continue
		}
		addr = append(addr, a)
	}
	return addr, err
}

// AddressByNamespacedName finds a specific ip address by namespace and name in a slice of addresses.
func AddressByNamespacedName(addresses []ipamv1.IPAddress, namespace, name string) *ipamv1.IPAddress {
	for _, a := range addresses {
		if a.Namespace == namespace && a.Name == name {
			return &a
		}
	}
	return nil
}

// FindFreeAddress returns the next free IP Address in a range based on a set of existing addresses.
func FindFreeAddress(poolIPSet *netipx.IPSet, inUseIPSet *netipx.IPSet) (netip.Addr, error) {
	for _, iprange := range poolIPSet.Ranges() {
		ip := iprange.From()
		for {
			if !inUseIPSet.Contains(ip) {
				return ip, nil
			}
			if ip == iprange.To() {
				break
			}
			ip = ip.Next()
		}
	}
	return netip.Addr{}, errors.New("no address available")
}

// PoolSpecToIPSet converts a pool spec to an IPSet. Reserved addresses will be
// omitted from the set depending on whether the
// `spec.AllocateReservedIPAddresses` flag is set.
func PoolSpecToIPSet(poolSpec *v1alpha2.InClusterIPPoolSpec) (*netipx.IPSet, error) {
	addressesIPSet, err := AddressesToIPSet(poolSpec.Addresses)
	if err != nil {
		return nil, err // should not happen, webhook validates pools for correctness.
	}

	builder := &netipx.IPSetBuilder{}
	builder.AddSet(addressesIPSet)

	if len(poolSpec.ExcludedAddresses) > 0 {
		excludedAddressesIPSet, err := AddressesToIPSet(poolSpec.ExcludedAddresses)
		if err != nil {
			return nil, err
		}

		builder.RemoveSet(excludedAddressesIPSet)
	}

	if !poolSpec.AllocateReservedIPAddresses {
		subnet := netip.PrefixFrom(addressesIPSet.Ranges()[0].From(), poolSpec.Prefix) // safe because of webhook validation
		subnetRange := netipx.RangeOfPrefix(subnet)
		builder.Remove(subnetRange.From()) // network addr in IPv4, anycast addr in IPv6
		if subnet.Addr().Is4() {
			builder.Remove(subnetRange.To()) // broadcast addr
		}
	}

	if poolSpec.Gateway != "" {
		gateway, err := netip.ParseAddr(poolSpec.Gateway)
		if err != nil {
			return nil, err // should not happen, webhook validates pools for correctness.
		}
		builder.Remove(gateway)
	}

	return builder.IPSet()
}

// AddressesToIPSet converts an array of addresses to an AddressesToIPSet
// addresses may be specified as individual IPs, CIDR ranges, or hyphenated IP
// ranges.
func AddressesToIPSet(addresses []string) (*netipx.IPSet, error) {
	builder := &netipx.IPSetBuilder{}
	for _, addressStr := range addresses {
		ipSet, err := AddressToIPSet(addressStr)
		if err != nil {
			return nil, err
		}
		builder.AddSet(ipSet)
	}
	return builder.IPSet()
}

// AddressToIPSet converts an addresses to an AddressesToIPSet addresses may be
// specified as individual IPs, CIDR ranges, or hyphenated IP ranges.
func AddressToIPSet(addressStr string) (*netipx.IPSet, error) {
	builder := &netipx.IPSetBuilder{}

	if strings.Contains(addressStr, "-") {
		addrRange, err := netipx.ParseIPRange(addressStr)
		if err != nil {
			return nil, err
		}
		builder.AddRange(addrRange)
	} else if strings.Contains(addressStr, "/") {
		prefix, err := netip.ParsePrefix(addressStr)
		if err != nil {
			return nil, err
		}
		builder.AddPrefix(prefix)
	} else {
		addr, err := netip.ParseAddr(addressStr)
		if err != nil {
			return nil, err
		}
		builder.Add(addr)
	}

	return builder.IPSet()
}

// IPSetCount returns the number of IPs contained in the given IPSet.
// This function returns type int, which is much smaller than
// the possible size of a range, which could be 2^128 IPs.
// When an IPSet's count is would be larger than an int, math.MaxInt
// is returned instead.
func IPSetCount(ipSet *netipx.IPSet) int {
	if ipSet == nil {
		return 0
	}

	total := big.NewInt(0)
	for _, iprange := range ipSet.Ranges() {
		total.Add(
			total,
			big.NewInt(0).Sub(
				big.NewInt(0).SetBytes(iprange.To().AsSlice()),
				big.NewInt(0).SetBytes(iprange.From().AsSlice()),
			),
		)
		// Subtracting To and From misses that one of those is a valid IP
		total.Add(total, big.NewInt(1))
	}

	// If total is greater than Uint64, Uint64() will return 0
	// We want to display MaxInt if the value overflows what int can contain
	if total.IsInt64() && total.Uint64() <= uint64(math.MaxInt) {
		return int(total.Uint64()) //nolint:gosec // [G115] conversion is safe with check beforehand
	}
	return math.MaxInt
}

// AddressStrParses checks to see that the addresss string is one of
// a valid single IP address, a hyphonated IP range, or a Prefix.
func AddressStrParses(addressStr string) bool {
	_, err := AddressToIPSet(addressStr)
	return err == nil
}
