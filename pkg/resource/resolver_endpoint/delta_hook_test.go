package resolver_endpoint

import (
	"testing"

	svcapitypes "github.com/aws-controllers-k8s/route53resolver-controller/apis/v1alpha1"
	ackv1alpha1 "github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1"
	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"
)

// deltaHookAddr builds a Spec.IPAddresses entry. An empty ip or subnetID leaves
// that pointer nil, which is what AWS omits and what a user may not have set.
func deltaHookAddr(subnetID, ip string) *svcapitypes.IPAddressRequest {
	addr := &svcapitypes.IPAddressRequest{}
	if subnetID != "" {
		addr.SubnetID = &subnetID
	}
	if ip != "" {
		addr.IP = &ip
	}
	return addr
}

// deltaHookRes wraps an address list in a resource, leaving every other Spec
// field zero so newResourceDelta can only report Spec.IPAddresses.
func deltaHookRes(addrs ...*svcapitypes.IPAddressRequest) *resource {
	return &resource{ko: &svcapitypes.ResolverEndpoint{
		Spec: svcapitypes.ResolverEndpointSpec{IPAddresses: addrs},
	}}
}

// TestNewResourceDeltaIPAddresses drives the real generated newResourceDelta, so
// it covers the generator.yaml wiring (compare.is_ignored plus
// delta_post_compare) and not just the comparison helper.
func TestNewResourceDeltaIPAddresses(t *testing.T) {
	withRef := deltaHookAddr("subnet-a", "")
	withRef.SubnetRef = &ackv1alpha1.AWSResourceReferenceWrapper{
		From: &ackv1alpha1.AWSResourceReference{Name: func() *string { n := "subnet-a-cr"; return &n }()},
	}

	tests := []struct {
		name    string
		desired *resource
		latest  *resource
		want    bool
	}{
		{
			name:    "same subnets in a different order, server-filled IPs",
			desired: deltaHookRes(deltaHookAddr("subnet-a", "10.0.0.5"), deltaHookAddr("subnet-b", "10.0.1.5")),
			latest:  deltaHookRes(deltaHookAddr("subnet-b", "10.0.1.5"), deltaHookAddr("subnet-a", "10.0.0.5")),
			want:    false,
		},
		{
			name:    "desired leaves IP unset, AWS assigned one",
			desired: deltaHookRes(deltaHookAddr("subnet-a", ""), deltaHookAddr("subnet-b", "")),
			latest:  deltaHookRes(deltaHookAddr("subnet-a", "10.0.0.5"), deltaHookAddr("subnet-b", "10.0.1.5")),
			want:    false,
		},
		{
			name:    "desired carries SubnetRef, observed never echoes it",
			desired: deltaHookRes(withRef, deltaHookAddr("subnet-b", "")),
			latest:  deltaHookRes(deltaHookAddr("subnet-a", "10.0.0.5"), deltaHookAddr("subnet-b", "10.0.1.5")),
			want:    false,
		},
		{
			name:    "adopted spec matches observed exactly",
			desired: deltaHookRes(deltaHookAddr("subnet-a", "10.0.0.5"), deltaHookAddr("subnet-b", "10.0.1.5")),
			latest:  deltaHookRes(deltaHookAddr("subnet-a", "10.0.0.5"), deltaHookAddr("subnet-b", "10.0.1.5")),
			want:    false,
		},
		{
			name:    "duplicate subnet IDs with unchanged membership",
			desired: deltaHookRes(deltaHookAddr("subnet-a", ""), deltaHookAddr("subnet-a", "")),
			latest:  deltaHookRes(deltaHookAddr("subnet-a", "10.0.0.5")),
			want:    false,
		},
		{
			name:    "both empty",
			desired: deltaHookRes(),
			latest:  deltaHookRes(),
			want:    false,
		},
		{
			name:    "desired adds a subnet",
			desired: deltaHookRes(deltaHookAddr("subnet-a", ""), deltaHookAddr("subnet-b", ""), deltaHookAddr("subnet-c", "")),
			latest:  deltaHookRes(deltaHookAddr("subnet-a", ""), deltaHookAddr("subnet-b", "")),
			want:    true,
		},
		{
			name:    "observed still holds an address awaiting deferred removal",
			desired: deltaHookRes(deltaHookAddr("subnet-b1", ""), deltaHookAddr("subnet-b2", "")),
			latest:  deltaHookRes(deltaHookAddr("subnet-a1", ""), deltaHookAddr("subnet-a2", ""), deltaHookAddr("subnet-b1", ""), deltaHookAddr("subnet-b2", "")),
			want:    true,
		},
		{
			name:    "one subnet replaced, same count",
			desired: deltaHookRes(deltaHookAddr("subnet-a", ""), deltaHookAddr("subnet-c", "")),
			latest:  deltaHookRes(deltaHookAddr("subnet-a", ""), deltaHookAddr("subnet-b", "")),
			want:    true,
		},
		{
			name:    "drift appears after adoption",
			desired: deltaHookRes(deltaHookAddr("subnet-a", "")),
			latest:  deltaHookRes(deltaHookAddr("subnet-a", ""), deltaHookAddr("subnet-b", "")),
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			delta := newResourceDelta(tt.desired, tt.latest)
			if got := delta.DifferentAt("Spec.IPAddresses"); got != tt.want {
				t.Fatalf("DifferentAt(Spec.IPAddresses) = %v, want %v", got, tt.want)
			}
			// Only IPAddresses is populated, so nothing else may differ.
			if !tt.want && len(delta.Differences) != 0 {
				t.Fatalf("expected an empty delta, got %d difference(s)", len(delta.Differences))
			}
		})
	}
}

// TestCustomPostCompareNilSafety exercises the hook directly, because
// newResourceDelta dereferences ko in the generated comparisons that run before
// the hook is reached.
func TestCustomPostCompareNilSafety(t *testing.T) {
	nilSubnet := &svcapitypes.IPAddressRequest{IP: func() *string { s := "10.0.0.5"; return &s }()}

	tests := []struct {
		name string
		a    *resource
		b    *resource
		want bool
	}{
		{name: "both resources nil", a: nil, b: nil, want: false},
		{name: "desired nil", a: nil, b: deltaHookRes(deltaHookAddr("subnet-a", "")), want: false},
		{name: "observed nil", a: deltaHookRes(deltaHookAddr("subnet-a", "")), b: nil, want: false},
		{name: "ko nil on both", a: &resource{}, b: &resource{}, want: false},
		{
			name: "nil list entry is skipped",
			a:    deltaHookRes(nil, deltaHookAddr("subnet-a", "")),
			b:    deltaHookRes(deltaHookAddr("subnet-a", "")),
			want: false,
		},
		{
			name: "entry with no subnet ID is skipped",
			a:    deltaHookRes(nilSubnet, deltaHookAddr("subnet-a", "")),
			b:    deltaHookRes(deltaHookAddr("subnet-a", "")),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			delta := ackcompare.NewDelta()
			customPostCompare(delta, tt.a, tt.b)
			if got := delta.DifferentAt("Spec.IPAddresses"); got != tt.want {
				t.Fatalf("DifferentAt(Spec.IPAddresses) = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestIPAddressSubnetIDs pins the set semantics the comparison relies on:
// duplicates collapse and entries without a subnet ID are dropped, matching
// GetIPAddressDifference.
func TestIPAddressSubnetIDs(t *testing.T) {
	got := ipAddressSubnetIDs([]*svcapitypes.IPAddressRequest{
		deltaHookAddr("subnet-a", "10.0.0.5"),
		deltaHookAddr("subnet-a", "10.0.0.6"),
		deltaHookAddr("subnet-b", ""),
		deltaHookAddr("", "10.0.2.5"),
		nil,
	})
	if len(got) != 2 {
		t.Fatalf("got %d subnet ID(s), want 2: %v", len(got), got)
	}
	for _, want := range []string{"subnet-a", "subnet-b"} {
		if _, ok := got[want]; !ok {
			t.Fatalf("missing %q in %v", want, got)
		}
	}
	if got := ipAddressSubnetIDs(nil); len(got) != 0 {
		t.Fatalf("nil list produced %d entries", len(got))
	}
}
