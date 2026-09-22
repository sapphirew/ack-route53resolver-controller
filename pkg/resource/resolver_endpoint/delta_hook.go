package resolver_endpoint

import (
	"maps"

	svcapitypes "github.com/aws-controllers-k8s/route53resolver-controller/apis/v1alpha1"
	ackcompare "github.com/aws-controllers-k8s/runtime/pkg/compare"
)

// customPostCompare compares Spec.IPAddresses as a set of subnet IDs, the same
// identity GetIPAddressDifference uses. The generated order-sensitive DeepEqual
// reported a difference no update could resolve, because AWS returns the
// addresses in an order of its choosing, fills in an IP the user may have left
// unset, and never echoes SubnetRef. An entry with no subnet ID is skipped, as
// it is by GetIPAddressDifference.
func customPostCompare(
	delta *ackcompare.Delta,
	a *resource,
	b *resource,
) {
	if a == nil || b == nil || a.ko == nil || b.ko == nil {
		return
	}
	if !maps.Equal(
		ipAddressSubnetIDs(a.ko.Spec.IPAddresses),
		ipAddressSubnetIDs(b.ko.Spec.IPAddresses),
	) {
		delta.Add("Spec.IPAddresses", a.ko.Spec.IPAddresses, b.ko.Spec.IPAddresses)
	}
}

// ipAddressSubnetIDs collects an address list's non-nil subnet IDs, the only
// field GetIPAddressDifference matches on.
func ipAddressSubnetIDs(
	ipAddresses []*svcapitypes.IPAddressRequest,
) map[string]struct{} {
	subnetIDs := make(map[string]struct{}, len(ipAddresses))
	for _, ipa := range ipAddresses {
		if ipa == nil || ipa.SubnetID == nil {
			continue
		}
		subnetIDs[*ipa.SubnetID] = struct{}{}
	}
	return subnetIDs
}
