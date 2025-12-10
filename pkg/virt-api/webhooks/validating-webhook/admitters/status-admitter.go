package admitters

import (
	"context"
	"fmt"

	admissionv1 "k8s.io/api/admission/v1"

	webhooks2 "kubevirt.io/kubevirt/pkg/virt-api/webhooks"

	"kubevirt.io/kubevirt/pkg/util/webhooks"
)

type StatusAdmitter struct {
	VmsAdmitter *VMsAdmitter
}

func log(fmtter string, vals ...any) {
	fmt.Printf("[debug]"+fmtter+"\n", vals...)
}

func (s *StatusAdmitter) Admit(ctx context.Context, ar *admissionv1.AdmissionReview) *admissionv1.AdmissionResponse {

	log("In StatusAdmitter.Admit, validating status...")

	if resp := webhooks.ValidateStatus(ar.Request.Object.Raw); resp != nil {
		log("something returned from ValidateStatus...")
		return resp
	}

	if webhooks.ValidateRequestResource(ar.Request.Resource, webhooks2.VirtualMachineGroupVersionResource.Group, webhooks2.VirtualMachineGroupVersionResource.Resource) {
		log("need validate resource...")
		result := s.VmsAdmitter.AdmitStatus(ctx, ar)
		log("validate resource result %v", result)
		return result
	}

	log("all good the request is allowed")

	reviewResponse := admissionv1.AdmissionResponse{}
	reviewResponse.Allowed = true
	return &reviewResponse
}
