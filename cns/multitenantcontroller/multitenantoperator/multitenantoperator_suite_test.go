package multitenantoperator

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestMultitenantoperator(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Multitenantoperator Suite")
}
