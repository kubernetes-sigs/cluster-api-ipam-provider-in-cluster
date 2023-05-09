package webhooks

import (
	"context"
	"testing"

	"github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

var ctx = ctrl.SetupSignalHandler()

// customDefaulterValidator interface is for objects that define both custom defaulting
// and custom validating webhooks.
type customDefaulterValidator interface {
	webhook.CustomDefaulter
	webhook.CustomValidator
}

// customDefaultValidateTest returns a new testing function to be used in tests to
// make sure custom defaulting webhooks also pass validation tests on create,
// update and delete.
func customDefaultValidateTest(ctx context.Context, obj runtime.Object, webhook customDefaulterValidator) func(*testing.T) {
	return func(t *testing.T) {
		t.Helper()

		t.Run("validate-on-create", func(t *testing.T) {
			g := gomega.NewWithT(t)
			createCopy := obj.DeepCopyObject()
			g.Expect(webhook.Default(ctx, createCopy)).To(gomega.Succeed())
			g.Expect(webhook.ValidateCreate(ctx, createCopy)).To(gomega.Succeed(), "should pass validation")
		})
		t.Run("validate-on-update", func(t *testing.T) {
			g := gomega.NewWithT(t)
			updateCopy := obj.DeepCopyObject()
			updatedCopy := obj.DeepCopyObject()
			g.Expect(webhook.Default(ctx, updatedCopy)).To(gomega.Succeed())
			g.Expect(webhook.Default(ctx, updateCopy)).To(gomega.Succeed())
			g.Expect(webhook.ValidateUpdate(ctx, updateCopy, updatedCopy)).To(gomega.Succeed(), "should pass validation")
		})
		t.Run("validate-on-delete", func(t *testing.T) {
			g := gomega.NewWithT(t)
			deleteCopy := obj.DeepCopyObject()
			g.Expect(webhook.Default(ctx, deleteCopy)).To(gomega.Succeed())
			g.Expect(webhook.ValidateDelete(ctx, deleteCopy)).To(gomega.Succeed(), "should pass validation")
		})
	}
}
