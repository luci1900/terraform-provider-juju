package tests

import (
	"testing"
)

func TestTemporal(t *testing.T) {
	teardown := setup("./temporal")
	defer teardown()
}
