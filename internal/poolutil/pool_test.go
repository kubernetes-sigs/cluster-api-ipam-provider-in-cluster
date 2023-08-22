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

package poolutil

import (
	"math"
	"net/netip"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go4.org/netipx"

	"sigs.k8s.io/cluster-api-ipam-provider-in-cluster/api/v1alpha2"
)

var _ = Describe("PoolSpecToIPSet", func() {
	It("converts a pool spec to a set", func() {
		spec := &v1alpha2.InClusterIPPoolSpec{
			Gateway: "192.168.0.1",
			Prefix:  24,
			Addresses: []string{
				"192.168.0.2",
				"192.168.0.3-192.168.0.4",
				"192.168.0.6/31",
			},
		}
		ipSet, err := PoolSpecToIPSet(spec)
		Expect(err).ToNot(HaveOccurred())
		Expect(ipSet.Contains(mustParse("192.168.0.2"))).To(BeTrue())
		Expect(ipSet.Contains(mustParse("192.168.0.3"))).To(BeTrue())
		Expect(ipSet.Contains(mustParse("192.168.0.4"))).To(BeTrue())
		Expect(ipSet.Contains(mustParse("192.168.0.6"))).To(BeTrue())
		Expect(ipSet.Contains(mustParse("192.168.0.7"))).To(BeTrue())
		Expect(IPSetCount(ipSet)).To(Equal(5))
	})
	It("excludes the 'network' address from the IPSet", func() {
		spec := &v1alpha2.InClusterIPPoolSpec{
			Gateway: "192.168.0.1",
			Prefix:  24,
			Addresses: []string{
				"192.168.0.0",
				"192.168.0.3-192.168.0.4",
				"192.168.0.6/31",
			},
		}
		ipSet, err := PoolSpecToIPSet(spec)
		Expect(err).ToNot(HaveOccurred())
		Expect(ipSet.Contains(mustParse("192.168.0.0"))).To(BeFalse())
		Expect(IPSetCount(ipSet)).To(Equal(4))
	})
	It("excludes the broadcast address from the IPSet", func() {
		spec := &v1alpha2.InClusterIPPoolSpec{
			Gateway: "192.168.0.1",
			Prefix:  16,
			Addresses: []string{
				"192.168.1.0",
				"192.168.0.255",
				"192.168.0.3-192.168.0.4",
				"192.168.0.6/31",
				"192.168.255.255",
			},
		}
		ipSet, err := PoolSpecToIPSet(spec)
		Expect(err).ToNot(HaveOccurred())
		Expect(ipSet.Contains(mustParse("192.168.1.0"))).To(BeTrue())
		Expect(ipSet.Contains(mustParse("192.168.0.255"))).To(BeTrue())
		Expect(ipSet.Contains(mustParse("192.168.255.255"))).To(BeFalse())
		Expect(IPSetCount(ipSet)).To(Equal(6))
	})
	It("excludes the gateway address from the IPSet", func() {
		spec := &v1alpha2.InClusterIPPoolSpec{
			Gateway: "192.168.0.1",
			Prefix:  24,
			Addresses: []string{
				"192.168.0.1",
				"192.168.0.3-192.168.0.4",
				"192.168.0.6/31",
			},
		}
		ipSet, err := PoolSpecToIPSet(spec)
		Expect(err).ToNot(HaveOccurred())
		Expect(ipSet.Contains(mustParse("192.168.0.1"))).To(BeFalse())
		Expect(IPSetCount(ipSet)).To(Equal(4))
	})
	Context("when the allocateReservedIPAddresses flag is true", func() {
		var ipSet *netipx.IPSet
		BeforeEach(func() {
			spec := &v1alpha2.InClusterIPPoolSpec{
				Gateway:                     "192.168.0.1",
				Prefix:                      24,
				AllocateReservedIPAddresses: true,
				Addresses: []string{
					"192.168.0.0",
					"192.168.0.1",
					"192.168.0.3-192.168.0.4",
					"192.168.0.6/31",
					"192.168.0.255",
				},
			}
			var err error
			ipSet, err = PoolSpecToIPSet(spec)
			Expect(err).ToNot(HaveOccurred())
		})
		It("includes the network and broadcast addresses in the IPSet", func() {
			Expect(ipSet.Contains(mustParse("192.168.0.0"))).To(BeTrue())
			Expect(ipSet.Contains(mustParse("192.168.0.3"))).To(BeTrue())
			Expect(ipSet.Contains(mustParse("192.168.0.4"))).To(BeTrue())
			Expect(ipSet.Contains(mustParse("192.168.0.6"))).To(BeTrue())
			Expect(ipSet.Contains(mustParse("192.168.0.7"))).To(BeTrue())
			Expect(ipSet.Contains(mustParse("192.168.0.255"))).To(BeTrue())
			Expect(IPSetCount(ipSet)).To(Equal(6))
		})
		It("never includes the gateway when set", func() {
			Expect(ipSet.Contains(mustParse("192.168.0.1"))).To(BeFalse())
		})
	})
	Context("With excluded ipv4 addresses", func() {
		It("excludes ip addresses from the address pool", func() {
			spec := &v1alpha2.InClusterIPPoolSpec{
				Gateway: "192.168.0.1",
				Prefix:  24,
				Addresses: []string{
					"192.168.0.0-192.168.0.10",
				},
				ExcludedAddresses: []string{
					"192.168.0.5",
					"192.168.0.6",
					"192.168.0.7",
				},
			}
			ipSet, err := PoolSpecToIPSet(spec)
			Expect(err).ToNot(HaveOccurred())
			Expect(ipSet.Contains(mustParse("192.168.0.2"))).To(BeTrue())
			Expect(ipSet.Contains(mustParse("192.168.0.3"))).To(BeTrue())
			Expect(ipSet.Contains(mustParse("192.168.0.4"))).To(BeTrue())
			Expect(ipSet.Contains(mustParse("192.168.0.5"))).To(BeFalse())
			Expect(ipSet.Contains(mustParse("192.168.0.6"))).To(BeFalse())
			Expect(ipSet.Contains(mustParse("192.168.0.7"))).To(BeFalse())
			Expect(ipSet.Contains(mustParse("192.168.0.8"))).To(BeTrue())
			Expect(ipSet.Contains(mustParse("192.168.0.9"))).To(BeTrue())
			Expect(ipSet.Contains(mustParse("192.168.0.10"))).To(BeTrue())
			Expect(ipSet.Contains(mustParse("192.168.0.11"))).To(BeFalse())
			Expect(IPSetCount(ipSet)).To(Equal(6))
		})
		It("excludes ip subnet from the address pool", func() {
			spec := &v1alpha2.InClusterIPPoolSpec{
				Gateway: "192.168.0.1",
				Prefix:  24,
				Addresses: []string{
					"192.168.0.0/24",
				},
				ExcludedAddresses: []string{
					"192.168.0.128/25",
				},
			}
			ipSet, err := PoolSpecToIPSet(spec)
			Expect(err).ToNot(HaveOccurred())
			Expect(ipSet.ContainsRange(netipx.MustParseIPRange("192.168.0.2-192.168.0.127"))).To(BeTrue())
			Expect(ipSet.ContainsRange(netipx.MustParseIPRange("192.168.0.128-192.168.0.255"))).To(BeFalse())
			Expect(IPSetCount(ipSet)).To(Equal(126))
		})
		It("excludes ip range from the address pool", func() {
			spec := &v1alpha2.InClusterIPPoolSpec{
				Gateway: "192.168.0.1",
				Prefix:  24,
				Addresses: []string{
					"192.168.0.0/24",
				},
				ExcludedAddresses: []string{
					"192.168.0.2-192.168.0.20",
				},
			}
			ipSet, err := PoolSpecToIPSet(spec)
			Expect(err).ToNot(HaveOccurred())
			Expect(ipSet.ContainsRange(netipx.MustParseIPRange("192.168.0.2-192.168.0.20"))).To(BeFalse())
			Expect(ipSet.ContainsRange(netipx.MustParseIPRange("192.168.0.21-192.168.0.254"))).To(BeTrue())
			Expect(IPSetCount(ipSet)).To(Equal(234))
		})
	})

	Context("with IPv6 addresses", func() {
		It("converts a pool spec to a set", func() {
			spec := &v1alpha2.InClusterIPPoolSpec{
				Gateway: "fd01::1",
				Prefix:  120,
				Addresses: []string{
					"fd01::2",
					"fd01::3-fd01::4",
					"fd01::6/127",
				},
			}
			ipSet, err := PoolSpecToIPSet(spec)
			Expect(err).ToNot(HaveOccurred())
			Expect(ipSet.Contains(mustParse("fd01::2"))).To(BeTrue())
			Expect(ipSet.Contains(mustParse("fd01::3"))).To(BeTrue())
			Expect(ipSet.Contains(mustParse("fd01::4"))).To(BeTrue())
			Expect(ipSet.Contains(mustParse("fd01::6"))).To(BeTrue())
			Expect(ipSet.Contains(mustParse("fd01::7"))).To(BeTrue())
			Expect(IPSetCount(ipSet)).To(Equal(5))
		})
		It("excludes the anycast address from the IPSet", func() {
			spec := &v1alpha2.InClusterIPPoolSpec{
				Gateway: "fd01::1",
				Prefix:  120,
				Addresses: []string{
					"fd01::0",
					"fd01::3-fd01::4",
					"fd01::6/127",
				},
			}
			ipSet, err := PoolSpecToIPSet(spec)
			Expect(err).ToNot(HaveOccurred())
			Expect(ipSet.Contains(mustParse("fd01::0"))).To(BeFalse())
			Expect(IPSetCount(ipSet)).To(Equal(4))
		})
		It("includes the broadcast address from the IPSet", func() {
			spec := &v1alpha2.InClusterIPPoolSpec{
				Gateway: "fd01::1",
				Prefix:  112,
				Addresses: []string{
					"fd01::ffff",
					"fd01::1:0",
					"fd01::3-fd01::4",
					"fd01::6/127",
					"fd01::ffff:ffff",
				},
			}
			ipSet, err := PoolSpecToIPSet(spec)
			Expect(err).ToNot(HaveOccurred())
			Expect(ipSet.Contains(mustParse("fd01::1:0"))).To(BeTrue())
			Expect(ipSet.Contains(mustParse("fd01::ffff"))).To(BeTrue())
			Expect(ipSet.Contains(mustParse("fd01::ffff:ffff"))).To(BeTrue())
			Expect(IPSetCount(ipSet)).To(Equal(7))
		})
		It("excludes the gateway address from the IPSet", func() {
			spec := &v1alpha2.InClusterIPPoolSpec{
				Gateway: "fd01::1",
				Prefix:  120,
				Addresses: []string{
					"fd01::1",
					"fd01::3-fd01::4",
					"fd01::6/127",
				},
			}
			ipSet, err := PoolSpecToIPSet(spec)
			Expect(err).ToNot(HaveOccurred())
			Expect(ipSet.Contains(mustParse("fd01::1"))).To(BeFalse())
			Expect(IPSetCount(ipSet)).To(Equal(4))
		})
		Context("when the AllocateReservedIPAddresses flag is true", func() {
			var ipSet *netipx.IPSet
			BeforeEach(func() {
				spec := &v1alpha2.InClusterIPPoolSpec{
					Gateway:                     "fd01::1",
					Prefix:                      120,
					AllocateReservedIPAddresses: true,
					Addresses: []string{
						"fd01::0",
						"fd01::1",
						"fd01::3-fd01::4",
						"fd01::6/127",
						"fd01::ffff:ffff",
					},
				}
				var err error
				ipSet, err = PoolSpecToIPSet(spec)
				Expect(err).ToNot(HaveOccurred())
			})
			It("includes the anycast address in the IPSet", func() {
				Expect(ipSet.Contains(mustParse("fd01::0"))).To(BeTrue())
				Expect(ipSet.Contains(mustParse("fd01::3"))).To(BeTrue())
				Expect(ipSet.Contains(mustParse("fd01::4"))).To(BeTrue())
				Expect(ipSet.Contains(mustParse("fd01::6"))).To(BeTrue())
				Expect(ipSet.Contains(mustParse("fd01::7"))).To(BeTrue())
				Expect(ipSet.Contains(mustParse("fd01::ffff:ffff"))).To(BeTrue())
				Expect(IPSetCount(ipSet)).To(Equal(6))
			})
			It("never allocates the gateway address when set", func() {
				Expect(ipSet.Contains(mustParse("fd01::1"))).To(BeFalse())
			})
		})
	})
})

var _ = Describe("AddressesToIPSet", func() {
	It("converts the slice to an IPSet", func() {
		addressSlice := []string{
			"192.168.1.25",
			"192.168.1.26-192.168.1.28",
			"192.168.2.16/30",
		}
		ipSet, err := AddressesToIPSet(addressSlice)
		Expect(err).NotTo(HaveOccurred())

		ip, err := netip.ParseAddr("192.168.1.25")
		Expect(err).NotTo(HaveOccurred())
		Expect(ipSet.Contains(ip)).To(BeTrue())

		ip, err = netip.ParseAddr("192.168.1.26")
		Expect(err).NotTo(HaveOccurred())
		Expect(ipSet.Contains(ip)).To(BeTrue())

		ip, err = netip.ParseAddr("192.168.1.27")
		Expect(err).NotTo(HaveOccurred())
		Expect(ipSet.Contains(ip)).To(BeTrue())

		ip, err = netip.ParseAddr("192.168.1.28")
		Expect(err).NotTo(HaveOccurred())
		Expect(ipSet.Contains(ip)).To(BeTrue())

		ip, err = netip.ParseAddr("192.168.1.29")
		Expect(err).NotTo(HaveOccurred())
		Expect(ipSet.Contains(ip)).To(BeFalse())

		ip, err = netip.ParseAddr("192.168.2.15")
		Expect(err).NotTo(HaveOccurred())
		Expect(ipSet.Contains(ip)).To(BeFalse())

		ip, err = netip.ParseAddr("192.168.2.16")
		Expect(err).NotTo(HaveOccurred())
		Expect(ipSet.Contains(ip)).To(BeTrue())

		ip, err = netip.ParseAddr("192.168.2.17")
		Expect(err).NotTo(HaveOccurred())
		Expect(ipSet.Contains(ip)).To(BeTrue())

		ip, err = netip.ParseAddr("192.168.2.18")
		Expect(err).NotTo(HaveOccurred())
		Expect(ipSet.Contains(ip)).To(BeTrue())

		ip, err = netip.ParseAddr("192.168.2.19")
		Expect(err).NotTo(HaveOccurred())
		Expect(ipSet.Contains(ip)).To(BeTrue())

		ip, err = netip.ParseAddr("192.168.2.20")
		Expect(err).NotTo(HaveOccurred())
		Expect(ipSet.Contains(ip)).To(BeFalse())
	})

	It("handles combinations of IPv6 ranges, cidrs, and individual IPs", func() {
		addresses := []string{
			"fe80::10/127",
			"fe80::13-fe80::14",
			"fe80::16",
		}

		ipSet, err := AddressesToIPSet(addresses)
		Expect(err).NotTo(HaveOccurred())
		Expect(ipSet.Contains(mustParse("fe80::9"))).To(BeFalse())
		Expect(ipSet.Contains(mustParse("fe80::10"))).To(BeTrue())
		Expect(ipSet.Contains(mustParse("fe80::11"))).To(BeTrue())
		Expect(ipSet.Contains(mustParse("fe80::12"))).To(BeFalse())

		Expect(ipSet.Contains(mustParse("fe80::12"))).To(BeFalse())
		Expect(ipSet.Contains(mustParse("fe80::13"))).To(BeTrue())
		Expect(ipSet.Contains(mustParse("fe80::14"))).To(BeTrue())
		Expect(ipSet.Contains(mustParse("fe80::15"))).To(BeFalse())

		Expect(ipSet.Contains(mustParse("fe80::16"))).To(BeTrue())
	})

	It("returns and error when addresses is invalid", func() {
		addressSlice := []string{"192.168.1.25", "192.168.1.2612"}
		_, err := AddressesToIPSet(addressSlice)
		Expect(err).To(HaveOccurred())
		addressSlice = []string{"192.168.1.2-192.168.1.4-"}
		_, err = AddressesToIPSet(addressSlice)
		Expect(err).To(HaveOccurred())
		addressSlice = []string{"192.168.1.2-192.168.1.4-192.168.1.9"}
		_, err = AddressesToIPSet(addressSlice)
		Expect(err).To(HaveOccurred())
		addressSlice = []string{"192.168.1.4-192.168.1.2"}
		_, err = AddressesToIPSet(addressSlice)
		Expect(err).To(HaveOccurred())
		addressSlice = []string{"192.168.1.4/21-192.168.1.2"}
		_, err = AddressesToIPSet(addressSlice)
		Expect(err).To(HaveOccurred())
		addressSlice = []string{"192.168.1.4/-192.168.1.2"}
		_, err = AddressesToIPSet(addressSlice)
		Expect(err).To(HaveOccurred())
		addressSlice = []string{"192.168.1.4-192.168.1.2"}
		_, err = AddressesToIPSet(addressSlice)
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("FindFreeAddress", func() {
	var (
		freeIPSet *netipx.IPSet
		existing  *netipx.IPSet
	)

	type iprange struct {
		From string
		To   string
	}

	createIPSet := func(ipranges []iprange) *netipx.IPSet {
		builder := netipx.IPSetBuilder{}
		for _, r := range ipranges {
			first, err := netip.ParseAddr(r.From)
			Expect(err).NotTo(HaveOccurred())

			second, err := netip.ParseAddr(r.To)
			Expect(err).NotTo(HaveOccurred())

			builder.AddRange(netipx.IPRangeFrom(first, second))
		}

		ipSet, err := builder.IPSet()
		Expect(err).NotTo(HaveOccurred())

		return ipSet
	}

	BeforeEach(func() {
		freeIPSet = nil

		var err error
		existingIPSetBuilder := netipx.IPSetBuilder{}
		existing, err = existingIPSetBuilder.IPSet()
		Expect(err).NotTo(HaveOccurred())
	})

	When("there is a single IPRange", func() {
		BeforeEach(func() {
			freeIPSet = createIPSet([]iprange{
				{From: "10.0.0.10", To: "10.0.0.20"},
			})
		})

		It("returns the first address in that range", func() {
			ip, err := FindFreeAddress(freeIPSet, existing)
			Expect(err).NotTo(HaveOccurred())
			Expect(ip.String()).To(Equal("10.0.0.10"))
		})

		When("there are some addresses claimed", func() {
			BeforeEach(func() {
				existing = createIPSet([]iprange{
					{From: "10.0.0.10", To: "10.0.0.11"},
				})
			})

			It("returns a free address from that range", func() {
				ip, err := FindFreeAddress(freeIPSet, existing)
				Expect(err).NotTo(HaveOccurred())
				Expect(ip.String()).To(Equal("10.0.0.12"))
			})
		})

		When("only the last ip address is available", func() {
			BeforeEach(func() {
				existing = createIPSet([]iprange{
					{From: "10.0.0.10", To: "10.0.0.19"},
				})
			})

			It("returns that ip address", func() {
				ip, err := FindFreeAddress(freeIPSet, existing)
				Expect(err).NotTo(HaveOccurred())
				Expect(ip.String()).To(Equal("10.0.0.20"))
			})
		})

		When("there are no more available addresses in the range", func() {
			BeforeEach(func() {
				existing = createIPSet([]iprange{
					{From: "10.0.0.10", To: "10.0.0.20"},
				})
			})

			It("returns an error indicating no addresses are available", func() {
				_, err := FindFreeAddress(freeIPSet, existing)
				Expect(err).To(MatchError(ContainSubstring("no address available")))
			})
		})
	})

	When("there is a ip address range of a single ip", func() {
		BeforeEach(func() {
			freeIPSet = createIPSet([]iprange{
				{From: "10.0.1.10", To: "10.0.1.10"},
			})
		})

		It("returns the single IP in that range", func() {
			ip, err := FindFreeAddress(freeIPSet, existing)
			Expect(err).NotTo(HaveOccurred())
			Expect(ip.String()).To(Equal("10.0.1.10"))
		})

		When("there are no more addresses available", func() {
			BeforeEach(func() {
				existing = createIPSet([]iprange{
					{From: "10.0.1.10", To: "10.0.1.10"},
				})
			})

			It("returns an error indicating no addresses are available", func() {
				_, err := FindFreeAddress(freeIPSet, existing)
				Expect(err).To(MatchError(ContainSubstring("no address available")))
			})
		})
	})

	When("there is are multiple IPRanges", func() {
		BeforeEach(func() {
			freeIPSet = createIPSet([]iprange{
				{From: "10.0.0.10", To: "10.0.0.15"},
				{From: "10.0.0.20", To: "10.0.0.20"},
			})
		})

		When("the first range is already completely allocated", func() {
			BeforeEach(func() {
				existing = createIPSet([]iprange{
					{From: "10.0.0.10", To: "10.0.0.15"},
				})
			})

			It("returns a free address from the next free range", func() {
				ip, err := FindFreeAddress(freeIPSet, existing)
				Expect(err).NotTo(HaveOccurred())
				Expect(ip.String()).To(Equal("10.0.0.20"))
			})
		})

		When("there are no more available addresses in the range", func() {
			BeforeEach(func() {
				existing = createIPSet([]iprange{
					{From: "10.0.0.10", To: "10.0.0.15"},
					{From: "10.0.0.20", To: "10.0.0.20"},
				})
			})

			It("returns an error indicating no addresses are available", func() {
				_, err := FindFreeAddress(freeIPSet, existing)
				Expect(err).To(MatchError(ContainSubstring("no address available")))
			})
		})
	})
})

var _ = Describe("AddressStrParses", func() {
	It("returns true for valid address strings", func() {
		Expect(AddressStrParses("192.168.1.2")).To(BeTrue())
		Expect(AddressStrParses("192.168.1.2-192.168.1.4")).To(BeTrue())
		Expect(AddressStrParses("192.168.1.2/24")).To(BeTrue())

		Expect(AddressStrParses("fe80::1")).To(BeTrue())
		Expect(AddressStrParses("fe80::1-fe80::3")).To(BeTrue())
		Expect(AddressStrParses("fe80::1/64")).To(BeTrue())
	})

	It("returns false for invalid address strings", func() {
		Expect(AddressStrParses("192.168.1.2.4")).To(BeFalse())
		Expect(AddressStrParses("192.168.1.4-192.168.1.1")).To(BeFalse())
		Expect(AddressStrParses("192.168.1.2/33")).To(BeFalse())

		Expect(AddressStrParses("fe80::zzzz")).To(BeFalse())
		Expect(AddressStrParses("fe80::3-fe80::1")).To(BeFalse())
		Expect(AddressStrParses("fe80::1/129")).To(BeFalse())
	})
})

var _ = DescribeTable("IPSetCount", func(expectedCount int, addresses ...string) {
	ipSet, err := AddressesToIPSet(addresses)
	Expect(err).NotTo(HaveOccurred())
	Expect(IPSetCount(ipSet)).To(Equal(expectedCount))
},
	Entry("no addresses", 0),
	Entry("one address", 1, "192.168.1.1"),
	Entry("two addresses", 2, "192.168.1.1", "192.168.1.2"),
	Entry("range of addresses", 16, "192.168.1.1-192.168.1.10", "192.168.1.20-192.168.1.25"),
	Entry("overlapping ranges of addresses", 15, "192.168.1.1-192.168.1.10", "192.168.1.5-192.168.1.15"),
	Entry("ipv4 CIDRs", 258, "192.168.1.1/24", "192.168.2.1/31"),
	Entry("range larger than uint64", math.MaxInt, "fe80::1/20"),
	Entry("range larger than int, but less than uint64", math.MaxInt, "fe80::1/65"),
	Entry("ipv6 CIDR", 4, "fe80::1/126"),
)

func mustParse(ipString string) netip.Addr {
	ip, err := netip.ParseAddr(ipString)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return ip
}
