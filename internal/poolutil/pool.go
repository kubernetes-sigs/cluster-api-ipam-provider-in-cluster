package poolutil

import (
	"context"
	"errors"

	"inet.af/netaddr"
	clusterv1exp "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func ListAddresses(ctx context.Context, poolName string, c client.Client) (*clusterv1exp.IPAddressList, error) {
	addresses := &clusterv1exp.IPAddressList{}
	err := c.List(ctx, addresses, &client.MatchingLabels{clusterv1exp.IPPoolLabel: poolName})
	return addresses, err
}

func AddressByName(addresses *clusterv1exp.IPAddressList, name string) *clusterv1exp.IPAddress {
	for _, a := range addresses.Items {
		if a.Name == name {
			return &a
		}
	}
	return nil
}

func IPAddressListToSet(list *clusterv1exp.IPAddressList, gateway string) (*netaddr.IPSet, error) {
	builder := netaddr.IPSetBuilder{}
	for _, a := range list.Items {
		addr, err := netaddr.ParseIP(a.Spec.Address)
		if err != nil {
			return nil, err
		}
		builder.Add(addr)
	}

	return builder.IPSet()
}

func FindFreeAddress(iprange netaddr.IPRange, existing *netaddr.IPSet) (netaddr.IP, error) {
	ip := iprange.From().Next()
	for {
		if !existing.Contains(ip) {
			return ip, nil
		}
		ip = ip.Next()
		if ip == iprange.To() {
			return netaddr.IP{}, errors.New("no address available")
		}
	}
}
