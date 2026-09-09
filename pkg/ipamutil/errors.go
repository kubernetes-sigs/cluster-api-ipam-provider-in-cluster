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

package ipamutil

// PoolExhaustedError signals that the IP pool has no available addresses.
// ClaimHandler implementations should return this error (wrapping the original)
// so the reconciler can set the PoolExhausted condition on the IPAddressClaim.
type PoolExhaustedError struct {
	Err error
}

func (e *PoolExhaustedError) Error() string { return e.Err.Error() }
func (e *PoolExhaustedError) Unwrap() error { return e.Err }
