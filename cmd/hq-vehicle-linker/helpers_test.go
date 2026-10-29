package main

import (
	"os"
	"testing"

	"github.com/GekkoGear/portal-armory/hqlink"
)

func hqlinkFind(t *testing.T) hqlink.Report {
	t.Helper()
	b, err := os.ReadFile("../../hqlink/testdata/small_pretty.json")
	if err != nil {
		t.Fatal(err)
	}
	rep, err := hqlink.Find(b)
	if err != nil {
		t.Fatal(err)
	}
	return rep
}
