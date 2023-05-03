// Package poolutil implements utility functions to manage a pool of IP addresses.
package poolutil

import (
	"context"
	"errors"
	"net/netip"
	"strings"

	"go4.org/netipx"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ipamv1 "sigs.k8s.io/cluster-api/exp/ipam/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/telekom/cluster-api-ipam-provider-in-cluster/api/v1alpha1"
	"github.com/telekom/cluster-api-ipam-provider-in-cluster/internal/index"
)

// ListAddressesInUse fetches all IPAddresses belonging to the specified pool.
// Note: requires `index.ipAddressByCombinedPoolRef` to be set up.
func ListAddressesInUse(ctx context.Context, c client.Client, namespace string, poolRef corev1.TypedLocalObjectReference) ([]ipamv1.IPAddress, error) {
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

// IPPoolSpecToIPSet converts poolSpec to a set of IP.
func IPPoolSpecToIPSet(poolSpec *v1alpha1.InClusterIPPoolSpec) (*netipx.IPSet, error) {
	if len(poolSpec.Addresses) > 0 {
		return AddressesToIPSet(poolSpec.Addresses)
	}

	builder := &netipx.IPSetBuilder{}

	start, err := netip.ParseAddr(poolSpec.First)
	if err != nil {
		return nil, err
	}

	end, err := netip.ParseAddr(poolSpec.Last)
	if err != nil {
		return nil, err
	}

	builder.AddRange(netipx.IPRangeFrom(start, end))

	return builder.IPSet()
}

// AddressStrParses checks to see that the addresss string is one of
// a valid single IP address, a hyphonated IP range, or a Prefix.
func AddressStrParses(addressStr string) bool {
	_, err := AddressToIPSet(addressStr)
	return err == nil
}
