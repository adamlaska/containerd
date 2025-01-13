/*
   Copyright The containerd Authors.

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

package platforms

import (
	"strconv"
	"strings"

	specs "github.com/opencontainers/image-spec/specs-go/v1"
)

// MatchComparer is able to match and compare platforms to
// filter and sort platforms.
type MatchComparer interface {
	Matcher

	Less(specs.Platform, specs.Platform) bool
}

// platformVector returns an (ordered) vector of appropriate specs.Platform
// objects to try matching for the given platform object (see platforms.Only).
func platformVector(platform specs.Platform) []specs.Platform {
	vector := []specs.Platform{platform}

	switch platform.Architecture {
	case "amd64":
		if amd64Version, err := strconv.Atoi(strings.TrimPrefix(platform.Variant, "v")); err == nil && amd64Version > 1 {
			for amd64Version--; amd64Version >= 1; amd64Version-- {
				vector = append(vector, specs.Platform{
					Architecture: platform.Architecture,
					OS:           platform.OS,
					OSVersion:    platform.OSVersion,
					OSFeatures:   platform.OSFeatures,
					Variant:      "v" + strconv.Itoa(amd64Version),
				})
			}
		}
		vector = append(vector, specs.Platform{
			Architecture: "386",
			OS:           platform.OS,
			OSVersion:    platform.OSVersion,
			OSFeatures:   platform.OSFeatures,
		})
	case "arm":
		if armVersion, err := strconv.Atoi(strings.TrimPrefix(platform.Variant, "v")); err == nil && armVersion > 5 {
			for armVersion--; armVersion >= 5; armVersion-- {
				vector = append(vector, specs.Platform{
					Architecture: platform.Architecture,
					OS:           platform.OS,
					OSVersion:    platform.OSVersion,
					OSFeatures:   platform.OSFeatures,
					Variant:      "v" + strconv.Itoa(armVersion),
				})
			}
		}
	case "arm64":
		variant := platform.Variant
		if variant == "" {
			variant = "v8"
		}

		majorVariant, minorVariant, hasMinor := strings.Cut(variant, ".")
		if armMajor, err := strconv.Atoi(strings.TrimPrefix(majorVariant, "v")); err == nil && armMajor >= 8 {
			armMinor := 0
			if len(variant) == 4 {
				if minor, err := strconv.Atoi(minorVariant); err == nil && hasMinor {
					armMinor = minor
				}
			}

			if armMajor == 9 {
				for minor := armMinor - 1; minor >= 0; minor-- {
					arm64Variant := "v" + strconv.Itoa(armMajor) + "." + strconv.Itoa(minor)
					if minor == 0 {
						arm64Variant = "v" + strconv.Itoa(armMajor)
					}
					vector = append(vector, specs.Platform{
						Architecture: platform.Architecture,
						OS:           platform.OS,
						OSVersion:    platform.OSVersion,
						OSFeatures:   platform.OSFeatures,
						Variant:      arm64Variant,
					})
				}

				// v9.0 diverged from v8.5, meaning that v9.x is compatible with v8.{x+5} until v9.4/v8.9
				armMinor = armMinor + 5
				if armMinor > 9 {
					armMinor = 9
				}
				armMajor = 8
				vector = append(vector, specs.Platform{
					Architecture: platform.Architecture,
					OS:           platform.OS,
					OSVersion:    platform.OSVersion,
					OSFeatures:   platform.OSFeatures,
					Variant:      "v8." + strconv.Itoa(armMinor),
				})
			}

			for minor := armMinor - 1; minor >= 0; minor-- {
				arm64Variant := "v" + strconv.Itoa(armMajor) + "." + strconv.Itoa(minor)
				if minor == 0 {
					arm64Variant = "v" + strconv.Itoa(armMajor)
				}
				vector = append(vector, specs.Platform{
					Architecture: platform.Architecture,
					OS:           platform.OS,
					OSVersion:    platform.OSVersion,
					OSFeatures:   platform.OSFeatures,
					Variant:      arm64Variant,
				})
			}
		}

		// All arm64/v8.x and arm64/v9.x are compatible with arm/v8 (32-bits) and below.
		// There's no arm64 v9 variant, so it's normalized to v8.
		if strings.HasPrefix(variant, "v8.") || strings.HasPrefix(variant, "v9.") {
			variant = "v8"
		}
		vector = append(vector, platformVector(specs.Platform{
			Architecture: "arm",
			OS:           platform.OS,
			OSVersion:    platform.OSVersion,
			OSFeatures:   platform.OSFeatures,
			Variant:      variant,
		})...)
	}

	return vector
}

// Only returns a match comparer for a single platform
// using default resolution logic for the platform.
//
// For arm64/v9.x, will also match arm64/v9.{0..x-1} and arm64/v8.{0..x+5}
// For arm64/v8.x, will also match arm64/v8.{0..x-1}
// For arm/v8, will also match arm/v7, arm/v6 and arm/v5
// For arm/v7, will also match arm/v6 and arm/v5
// For arm/v6, will also match arm/v5
// For amd64, will also match 386
func Only(platform specs.Platform) MatchComparer {
	return Ordered(platformVector(Normalize(platform))...)
}

// OnlyStrict returns a match comparer for a single platform.
//
// Unlike Only, OnlyStrict does not match sub platforms.
// So, "arm/vN" will not match "arm/vM" where M < N,
// and "amd64" will not also match "386".
//
// OnlyStrict matches non-canonical forms.
// So, "arm64" matches "arm/64/v8".
func OnlyStrict(platform specs.Platform) MatchComparer {
	return Ordered(Normalize(platform))
}

// Ordered returns a platform MatchComparer which matches any of the platforms
// but orders them in order they are provided.
func Ordered(platforms ...specs.Platform) MatchComparer {
	matchers := make([]Matcher, len(platforms))
	for i := range platforms {
		matchers[i] = NewMatcher(platforms[i])
	}
	return orderedPlatformComparer{
		matchers: matchers,
	}
}

// Any returns a platform MatchComparer which matches any of the platforms
// with no preference for ordering.
func Any(platforms ...specs.Platform) MatchComparer {
	matchers := make([]Matcher, len(platforms))
	for i := range platforms {
		matchers[i] = NewMatcher(platforms[i])
	}
	return anyPlatformComparer{
		matchers: matchers,
	}
}

// All is a platform MatchComparer which matches all platforms
// with preference for ordering.
var All MatchComparer = allPlatformComparer{}

type orderedPlatformComparer struct {
	matchers []Matcher
}

func (c orderedPlatformComparer) Match(platform specs.Platform) bool {
	for _, m := range c.matchers {
		if m.Match(platform) {
			return true
		}
	}
	return false
}

func (c orderedPlatformComparer) Less(p1 specs.Platform, p2 specs.Platform) bool {
	for _, m := range c.matchers {
		p1m := m.Match(p1)
		p2m := m.Match(p2)
		if p1m && !p2m {
			return true
		}
		if p1m || p2m {
			return false
		}
	}
	return false
}

type anyPlatformComparer struct {
	matchers []Matcher
}

func (c anyPlatformComparer) Match(platform specs.Platform) bool {
	for _, m := range c.matchers {
		if m.Match(platform) {
			return true
		}
	}
	return false
}

func (c anyPlatformComparer) Less(p1, p2 specs.Platform) bool {
	var p1m, p2m bool
	for _, m := range c.matchers {
		if !p1m && m.Match(p1) {
			p1m = true
		}
		if !p2m && m.Match(p2) {
			p2m = true
		}
		if p1m && p2m {
			return false
		}
	}
	// If one matches, and the other does, sort match first
	return p1m && !p2m
}

type allPlatformComparer struct{}

func (allPlatformComparer) Match(specs.Platform) bool {
	return true
}

func (allPlatformComparer) Less(specs.Platform, specs.Platform) bool {
	return false
}
