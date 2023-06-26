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

package v1alpha2

// Hub marks InClusterIPPool as a conversion hub.
func (*InClusterIPPool) Hub() {}

// Hub marks GlobalInClusterIPPool as a conversion hub.
func (*GlobalInClusterIPPool) Hub() {}

// Hub marks InClusterIPPoolList as a conversion hub.
func (*InClusterIPPoolList) Hub() {}

// Hub marks GlobalInClusterIPPoolList as a conversion hub.
func (*GlobalInClusterIPPoolList) Hub() {}
