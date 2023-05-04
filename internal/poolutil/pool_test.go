package poolutil

import (
	"math"
	"net/netip"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go4.org/netipx"

	"github.com/telekom/cluster-api-ipam-provider-in-cluster/api/v1alpha1"
)

var _ = Describe("AddressListToSet", func() {
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

	It("errors on invalid addresses", func() {
		addressSlice := []string{
			"192.168.1.25",
			"192.168.1.2612",
		}
		_, err := AddressesToIPSet(addressSlice)
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

var _ = Describe("IPPoolSpecToIPSet", func() {
	Context("when the poolSpec has a list of addresses", func() {
		It("parses pools with start/end", func() {
			poolSpec := &v1alpha1.InClusterIPPoolSpec{
				First: "192.168.1.2",
				Last:  "192.168.1.4",
			}

			ipSet, err := IPPoolSpecToIPSet(poolSpec)
			Expect(err).NotTo(HaveOccurred())
			Expect(ipSet.Contains(mustParse("192.168.1.2"))).To(BeTrue())
			Expect(ipSet.Contains(mustParse("192.168.1.3"))).To(BeTrue())
			Expect(ipSet.Contains(mustParse("192.168.1.4"))).To(BeTrue())
			Expect(ipSet.Contains(mustParse("192.168.1.5"))).To(BeFalse())
		})

		It("parses single IPs", func() {
			poolSpec := &v1alpha1.InClusterIPPoolSpec{
				Addresses: []string{
					"192.168.1.2",
					"192.168.1.3",
				},
			}

			ipSet, err := IPPoolSpecToIPSet(poolSpec)
			Expect(err).NotTo(HaveOccurred())
			Expect(ipSet.Contains(mustParse("192.168.1.2"))).To(BeTrue())
			Expect(ipSet.Contains(mustParse("192.168.1.3"))).To(BeTrue())
			Expect(ipSet.Contains(mustParse("192.168.1.4"))).To(BeFalse())
		})

		It("parses IP ranges with a dash", func() {
			poolSpec := &v1alpha1.InClusterIPPoolSpec{
				Addresses: []string{
					"192.168.1.2-192.168.1.4",
					"192.168.1.7-192.168.1.9",
				},
			}

			ipSet, err := IPPoolSpecToIPSet(poolSpec)
			Expect(err).NotTo(HaveOccurred())
			Expect(ipSet.Contains(mustParse("192.168.1.2"))).To(BeTrue())
			Expect(ipSet.Contains(mustParse("192.168.1.3"))).To(BeTrue())
			Expect(ipSet.Contains(mustParse("192.168.1.4"))).To(BeTrue())
			Expect(ipSet.Contains(mustParse("192.168.1.5"))).To(BeFalse())
			Expect(ipSet.Contains(mustParse("192.168.1.6"))).To(BeFalse())
			Expect(ipSet.Contains(mustParse("192.168.1.7"))).To(BeTrue())
			Expect(ipSet.Contains(mustParse("192.168.1.8"))).To(BeTrue())
			Expect(ipSet.Contains(mustParse("192.168.1.9"))).To(BeTrue())
			Expect(ipSet.Contains(mustParse("192.168.1.10"))).To(BeFalse())
		})

		It("parses IP CIDRs", func() {
			poolSpec := &v1alpha1.InClusterIPPoolSpec{
				Addresses: []string{
					"192.168.1.8/29",
					"192.168.1.100/30",
				},
			}

			ipSet, err := IPPoolSpecToIPSet(poolSpec)
			Expect(err).NotTo(HaveOccurred())
			Expect(ipSet.Contains(mustParse("192.168.1.7"))).To(BeFalse())
			Expect(ipSet.Contains(mustParse("192.168.1.8"))).To(BeTrue())
			Expect(ipSet.Contains(mustParse("192.168.1.9"))).To(BeTrue())
			Expect(ipSet.Contains(mustParse("192.168.1.10"))).To(BeTrue())
			Expect(ipSet.Contains(mustParse("192.168.1.11"))).To(BeTrue())
			Expect(ipSet.Contains(mustParse("192.168.1.12"))).To(BeTrue())
			Expect(ipSet.Contains(mustParse("192.168.1.13"))).To(BeTrue())
			Expect(ipSet.Contains(mustParse("192.168.1.14"))).To(BeTrue())
			Expect(ipSet.Contains(mustParse("192.168.1.15"))).To(BeTrue())
			Expect(ipSet.Contains(mustParse("192.168.1.16"))).To(BeFalse())

			Expect(ipSet.Contains(mustParse("192.168.1.99"))).To(BeFalse())
			Expect(ipSet.Contains(mustParse("192.168.1.100"))).To(BeTrue())
			Expect(ipSet.Contains(mustParse("192.168.1.101"))).To(BeTrue())
			Expect(ipSet.Contains(mustParse("192.168.1.102"))).To(BeTrue())
			Expect(ipSet.Contains(mustParse("192.168.1.103"))).To(BeTrue())
			Expect(ipSet.Contains(mustParse("192.168.1.104"))).To(BeFalse())
		})

		It("handles combinations of ranges, cidrs, and individual IPs", func() {
			poolSpec := &v1alpha1.InClusterIPPoolSpec{
				Addresses: []string{
					"192.168.1.100/31",
					"192.168.1.2-192.168.1.4",
					"192.168.2.1",
				},
			}

			ipSet, err := IPPoolSpecToIPSet(poolSpec)
			Expect(err).NotTo(HaveOccurred())
			Expect(ipSet.Contains(mustParse("192.168.1.99"))).To(BeFalse())
			Expect(ipSet.Contains(mustParse("192.168.1.100"))).To(BeTrue())
			Expect(ipSet.Contains(mustParse("192.168.1.101"))).To(BeTrue())
			Expect(ipSet.Contains(mustParse("192.168.1.102"))).To(BeFalse())

			Expect(ipSet.Contains(mustParse("192.168.1.1"))).To(BeFalse())
			Expect(ipSet.Contains(mustParse("192.168.1.2"))).To(BeTrue())
			Expect(ipSet.Contains(mustParse("192.168.1.3"))).To(BeTrue())
			Expect(ipSet.Contains(mustParse("192.168.1.4"))).To(BeTrue())
			Expect(ipSet.Contains(mustParse("192.168.1.5"))).To(BeFalse())

			Expect(ipSet.Contains(mustParse("192.168.2.1"))).To(BeTrue())
		})

		It("handles combinations of IPv6 ranges, cidrs, and individual IPs", func() {
			poolSpec := &v1alpha1.InClusterIPPoolSpec{
				Addresses: []string{
					"fe80::10/127",
					"fe80::13-fe80::14",
					"fe80::16",
				},
			}

			ipSet, err := IPPoolSpecToIPSet(poolSpec)
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

		It("returns and error when spec is invalid", func() {
			poolSpec := &v1alpha1.InClusterIPPoolSpec{Addresses: []string{"192.168.1.2-192.168.1.4-"}}
			_, err := IPPoolSpecToIPSet(poolSpec)
			Expect(err).To(HaveOccurred())
			poolSpec = &v1alpha1.InClusterIPPoolSpec{Addresses: []string{"192.168.1.2-192.168.1.4-192.168.1.9"}}
			_, err = IPPoolSpecToIPSet(poolSpec)
			Expect(err).To(HaveOccurred())
			poolSpec = &v1alpha1.InClusterIPPoolSpec{Addresses: []string{"192.168.1.4-192.168.1.2"}}
			_, err = IPPoolSpecToIPSet(poolSpec)
			Expect(err).To(HaveOccurred())
			poolSpec = &v1alpha1.InClusterIPPoolSpec{Addresses: []string{"192.168.1.4/21-192.168.1.2"}}
			_, err = IPPoolSpecToIPSet(poolSpec)
			Expect(err).To(HaveOccurred())
			poolSpec = &v1alpha1.InClusterIPPoolSpec{Addresses: []string{"192.168.1.4/-192.168.1.2"}}
			_, err = IPPoolSpecToIPSet(poolSpec)
			Expect(err).To(HaveOccurred())
			poolSpec = &v1alpha1.InClusterIPPoolSpec{Addresses: []string{"192.168.1.4-192.168.1.2"}}
			_, err = IPPoolSpecToIPSet(poolSpec)
			Expect(err).To(HaveOccurred())
		})
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
