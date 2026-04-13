/*
Copyright 2026 The Kubernetes Authors.

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
	"errors"
	"fmt"
	"math"
	"math/big"
	"net/netip"
	"slices"

	"go4.org/netipx"
	ipamv1 "sigs.k8s.io/cluster-api/api/ipam/v1beta2"

	"sigs.k8s.io/cluster-api-ipam-provider-in-cluster/api/v1alpha2"
)

const (
	defaultAllocationPrefixLength = 64
)

// EffectiveAllocationPrefixLength returns the configured allocation prefix length
// or the default value when the field is not explicitly set.
func EffectiveAllocationPrefixLength(allocationPrefixLength int) int {
	if allocationPrefixLength == 0 {
		return defaultAllocationPrefixLength
	}
	return allocationPrefixLength
}

// PrefixPoolConfig contains input configuration for prefix allocation.
type PrefixPoolConfig struct {
	Prefixes               []string
	AllocationPrefixLength int
	Gateway                string
	ExcludedPrefixes       []string
}

// NewPrefixPoolConfig builds a PrefixPoolConfig from a pool spec.
func NewPrefixPoolConfig(spec *v1alpha2.InClusterPrefixPoolSpec) *PrefixPoolConfig {
	return &PrefixPoolConfig{
		Prefixes:               spec.Prefixes,
		AllocationPrefixLength: EffectiveAllocationPrefixLength(spec.AllocationPrefixLength),
		Gateway:                spec.Gateway,
		ExcludedPrefixes:       spec.ExcludedPrefixes,
	}
}

// FindFreePrefix returns the next free prefix based on pool config and existing allocations.
func FindFreePrefix(poolConfig *PrefixPoolConfig, inUseIPSet *netipx.IPSet) (netip.Prefix, error) {
	allocLen, err := validatedAllocationPrefixLength(poolConfig)
	if err != nil {
		return netip.Prefix{}, err
	}

	poolIPSet, err := PrefixesToIPSet(poolConfig.Prefixes)
	if err != nil {
		return netip.Prefix{}, err
	}

	blockedIPSet, err := prefixPoolBlockedIPSet(poolConfig, inUseIPSet)
	if err != nil {
		return netip.Prefix{}, err
	}

	for _, aggregate := range sortedPrefixes(poolIPSet.Prefixes()) {
		if freePrefix, ok := findFreePrefixInAggregate(aggregate, allocLen, blockedIPSet); ok {
			return freePrefix, nil
		}
	}

	return netip.Prefix{}, errors.New("no prefix available")
}

// PrefixCandidateCount returns the count of allocatable prefixes from the pool configuration.
func PrefixCandidateCount(poolConfig *PrefixPoolConfig) (int, error) {
	allocLen, err := validatedAllocationPrefixLength(poolConfig)
	if err != nil {
		return 0, err
	}

	poolIPSet, err := PrefixesToIPSet(poolConfig.Prefixes)
	if err != nil {
		return 0, err
	}

	blockedIPSet, err := prefixPoolBlockedIPSet(poolConfig, nil)
	if err != nil {
		return 0, err
	}

	total := 0
	for _, aggregate := range sortedPrefixes(poolIPSet.Prefixes()) {
		total = addIntCapped(total, countAllocatablePrefixesInAggregate(aggregate, allocLen, blockedIPSet))
	}
	return total, nil
}

// PrefixIsAllocatable returns true if a prefix is allocatable for the given pool configuration.
func PrefixIsAllocatable(prefix netip.Prefix, poolConfig *PrefixPoolConfig) bool {
	if poolConfig == nil || !prefix.IsValid() {
		return false
	}
	allocLen, err := validatedAllocationPrefixLength(poolConfig)
	if err != nil {
		return false
	}

	prefix = prefix.Masked()
	if prefix.Bits() != allocLen {
		return false
	}

	poolIPSet, err := PrefixesToIPSet(poolConfig.Prefixes)
	if err != nil {
		return false
	}

	if !poolIPSet.ContainsPrefix(prefix) {
		return false
	}

	blockedIPSet, err := prefixPoolBlockedIPSet(poolConfig, nil)
	if err != nil {
		return false
	}

	return !blockedIPSet.OverlapsPrefix(prefix)
}

func validatedAllocationPrefixLength(poolConfig *PrefixPoolConfig) (int, error) {
	if poolConfig == nil {
		return 0, errors.New("prefix pool config is required")
	}
	if poolConfig.AllocationPrefixLength <= 0 || poolConfig.AllocationPrefixLength > 128 {
		return 0, fmt.Errorf("invalid prefix length %d", poolConfig.AllocationPrefixLength)
	}
	return poolConfig.AllocationPrefixLength, nil
}

// PrefixFromIPAddress derives an allocation prefix from an IPAddress resource.
func PrefixFromIPAddress(address ipamv1.IPAddress) (netip.Prefix, error) {
	if address.Spec.Prefix == nil {
		return netip.Prefix{}, fmt.Errorf("address %s/%s has no prefix set", address.Namespace, address.Name)
	}

	prefix, err := netip.ParsePrefix(fmt.Sprintf("%s/%d", address.Spec.Address, *address.Spec.Prefix))
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("failed parsing address %s/%s as prefix: %w", address.Namespace, address.Name, err)
	}

	return prefix.Masked(), nil
}

// AddressesToAllocationPrefixSet converts an allocated IPAddress list to a range-based IPSet.
func AddressesToAllocationPrefixSet(addresses []ipamv1.IPAddress) (*netipx.IPSet, error) {
	builder := &netipx.IPSetBuilder{}
	for _, address := range addresses {
		prefix, err := PrefixFromIPAddress(address)
		if err != nil {
			return nil, err
		}
		builder.AddPrefix(prefix)
	}
	return builder.IPSet()
}

// PrefixesToIPSet converts prefix pool CIDR entries into an IPSet.
// All entries must be valid, network-aligned CIDRs.
func PrefixesToIPSet(prefixes []string) (*netipx.IPSet, error) {
	builder := &netipx.IPSetBuilder{}
	for _, entry := range prefixes {
		prefix, err := netip.ParsePrefix(entry)
		if err != nil {
			return nil, fmt.Errorf("invalid pool CIDR %q: %w", entry, err)
		}
		if prefix != prefix.Masked() {
			return nil, fmt.Errorf("pool CIDR %q is not network aligned", entry)
		}
		builder.AddPrefix(prefix)
	}
	return builder.IPSet()
}

func prefixPoolBlockedIPSet(poolConfig *PrefixPoolConfig, inUseIPSet *netipx.IPSet) (*netipx.IPSet, error) {
	builder := &netipx.IPSetBuilder{}

	if len(poolConfig.ExcludedPrefixes) > 0 {
		excludedIPSet, err := AddressesToIPSet(poolConfig.ExcludedPrefixes)
		if err != nil {
			return nil, err
		}
		builder.AddSet(excludedIPSet)
	}

	// Gateway is NOT added to the blocked set for prefix pools.
	// It is carried to IPAddress.Spec.Gateway for informational purposes
	// but does not prevent any prefix from being allocated.

	if inUseIPSet != nil {
		builder.AddSet(inUseIPSet)
	}

	return builder.IPSet()
}

func sortedPrefixes(prefixes []netip.Prefix) []netip.Prefix {
	sorted := append([]netip.Prefix(nil), prefixes...)
	slices.SortFunc(sorted, func(i, j netip.Prefix) int {
		if cmp := i.Addr().Compare(j.Addr()); cmp != 0 {
			return cmp
		}
		return i.Bits() - j.Bits()
	})
	return sorted
}

// findFreePrefixInAggregate performs a recursive binary search within an aggregate prefix
// to find the first available child prefix of exactly targetBits length.
// It splits the aggregate into halves, checking each sub-prefix in left-to-right order
// until a free (non-blocked) slot is found.
func findFreePrefixInAggregate(aggregate netip.Prefix, targetBits int, blockedIPSet *netipx.IPSet) (netip.Prefix, bool) {
	// The aggregate must be a valid prefix.
	if !aggregate.IsValid() {
		return netip.Prefix{}, false
	}

	bits := aggregate.Bits()
	// The aggregate must be at least as large as the target (fewer bits = larger prefix),
	// and the target must fit within the address family's bit length.
	if bits > targetBits || bits < 0 || targetBits > aggregate.Addr().BitLen() {
		return netip.Prefix{}, false
	}

	// The entire aggregate is blocked (fully covered by excluded or in-use prefixes).
	if blockedIPSet.ContainsPrefix(aggregate) {
		return netip.Prefix{}, false
	}

	// No part of the aggregate is blocked => take the first (lowest-address) child prefix.
	if !blockedIPSet.OverlapsPrefix(aggregate) {
		if bits == targetBits {
			return aggregate, true
		}
		return netip.PrefixFrom(aggregate.Addr(), targetBits).Masked(), true
	}

	// The aggregate is partially blocked but already at target size => unusable.
	if bits == targetBits {
		return netip.Prefix{}, false
	}

	// Split in half and recurse for deterministic first-fit allocation.
	left, right, ok := splitPrefix(aggregate)
	if !ok {
		return netip.Prefix{}, false
	}

	if prefix, found := findFreePrefixInAggregate(left, targetBits, blockedIPSet); found {
		return prefix, true
	}
	return findFreePrefixInAggregate(right, targetBits, blockedIPSet)
}

// countAllocatablePrefixesInAggregate mirrors the search logic of findFreePrefixInAggregate
// but instead of returning the first free prefix, it counts all allocatable child prefixes.
// It uses the same recursive binary splitting: fully blocked subtrees contribute 0,
// fully free subtrees contribute 2^(targetBits - bits), and partially blocked subtrees
// are split further until the target prefix length is reached.
func countAllocatablePrefixesInAggregate(aggregate netip.Prefix, targetBits int, blockedIPSet *netipx.IPSet) int {
	if !aggregate.IsValid() {
		return 0
	}

	bits := aggregate.Bits()
	// The aggregate must be at least as large as the target and within address family bounds.
	if bits > targetBits || bits < 0 || targetBits > aggregate.Addr().BitLen() {
		return 0
	}

	// The entire aggregate is blocked => zero allocatable prefixes.
	if blockedIPSet.ContainsPrefix(aggregate) {
		return 0
	}

	// No part of the aggregate is blocked => all 2^(targetBits-bits) child prefixes are free.
	if !blockedIPSet.OverlapsPrefix(aggregate) {
		return twoPowerCapped(targetBits - bits)
	}

	// Already at target size but partially blocked => unusable.
	if bits == targetBits {
		return 0
	}

	// Split and sum allocatable prefixes from both halves.
	left, right, ok := splitPrefix(aggregate)
	if !ok {
		return 0
	}

	leftCount := countAllocatablePrefixesInAggregate(left, targetBits, blockedIPSet)
	rightCount := countAllocatablePrefixesInAggregate(right, targetBits, blockedIPSet)
	return addIntCapped(leftCount, rightCount)
}

func splitPrefix(prefix netip.Prefix) (netip.Prefix, netip.Prefix, bool) {
	if !prefix.IsValid() {
		return netip.Prefix{}, netip.Prefix{}, false
	}

	bits := prefix.Bits()
	if bits >= prefix.Addr().BitLen() || bits < 0 {
		return netip.Prefix{}, netip.Prefix{}, false
	}

	left := netip.PrefixFrom(prefix.Addr(), bits+1).Masked()
	rightAddr := netipx.RangeOfPrefix(left).To().Next()
	if !rightAddr.IsValid() {
		return netip.Prefix{}, netip.Prefix{}, false
	}
	right := netip.PrefixFrom(rightAddr, bits+1).Masked()

	return left, right, true
}

func twoPowerCapped(exponent int) int {
	if exponent < 0 {
		return 0
	}
	if exponent == 0 {
		return 1
	}

	total := big.NewInt(1)
	total.Lsh(total, uint(exponent))

	maxInt := big.NewInt(int64(math.MaxInt))
	if total.Cmp(maxInt) > 0 {
		return math.MaxInt
	}
	return int(total.Int64())
}

func addIntCapped(current, add int) int {
	if current == math.MaxInt || add == math.MaxInt {
		return math.MaxInt
	}
	if add > math.MaxInt-current {
		return math.MaxInt
	}
	return current + add
}
