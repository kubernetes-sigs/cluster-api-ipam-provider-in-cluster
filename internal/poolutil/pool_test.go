package poolutil

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"inet.af/netaddr"
)

var _ = Describe("FindFreeAddress", func() {
	var (
		freeIPSet *netaddr.IPSet
		existing  *netaddr.IPSet
	)

	type iprange struct {
		From string
		To   string
	}

	createIPSet := func(ipranges []iprange) *netaddr.IPSet {
		builder := netaddr.IPSetBuilder{}
		for _, r := range ipranges {
			first, err := netaddr.ParseIP(r.From)
			Expect(err).NotTo(HaveOccurred())

			second, err := netaddr.ParseIP(r.To)
			Expect(err).NotTo(HaveOccurred())

			builder.AddRange(netaddr.IPRangeFrom(first, second))
		}

		ipSet, err := builder.IPSet()
		Expect(err).NotTo(HaveOccurred())

		return ipSet
	}

	BeforeEach(func() {
		freeIPSet = nil

		var err error
		existingIPSetBuilder := netaddr.IPSetBuilder{}
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
