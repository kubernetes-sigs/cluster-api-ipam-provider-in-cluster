package poolutil_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestPoolutil(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Poolutil Suite")
}
