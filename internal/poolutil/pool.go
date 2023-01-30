// Package poolutil implements utility functions to manage a pool of IP addresses.
package poolutil

import (
	"context"
	"errors"
	"fmt"

	"inet.af/netaddr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ipamv1 "sigs.k8s.io/cluster-api/exp/ipam/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/telekom/cluster-api-ipam-provider-in-cluster/internal/index"
)

// ListAddresses fetches all IPAddresses belonging to the specified pool.
// Note: requires `index.ipAddressByCombinedPoolRef` to be set up.
func ListAddresses(ctx context.Context, c client.Client, namespace string, poolRef corev1.TypedLocalObjectReference) ([]ipamv1.IPAddress, error) {
	addresses := &ipamv1.IPAddressList{}
	err := c.List(ctx, addresses,
		client.MatchingFields{
			index.IPAddressPoolRefCombinedField: index.IPAddressPoolRefValue(poolRef),
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

// AddressByName finds a specific ip address by name in a slice of addresses.
func AddressByName(addresses []ipamv1.IPAddress, name string) *ipamv1.IPAddress {
	for _, a := range addresses {
		if a.Name == name {
			return &a
		}
	}
	return nil
}

// IPAddressListToSet converts a slice of ip address resources into a set.
func IPAddressListToSet(list []ipamv1.IPAddress, gateway string) (*netaddr.IPSet, error) {
	builder := netaddr.IPSetBuilder{}
	for _, a := range list {
		addr, err := netaddr.ParseIP(a.Spec.Address)
		if err != nil {
			return nil, err
		}
		builder.Add(addr)
	}
	gw, err := netaddr.ParseIP(gateway)
	if err != nil {
		return nil, fmt.Errorf("failed to parse gateway ip: %w", err)
	}
	builder.Add(gw)

	return builder.IPSet()
}

// FindFreeAddress returns the next free IP Address in a range based on a set of existing addresses.
func FindFreeAddress(poolIPSet *netaddr.IPSet, inUseIPSet *netaddr.IPSet) (netaddr.IP, error) {
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
	return netaddr.IP{}, errors.New("no address available")
}
