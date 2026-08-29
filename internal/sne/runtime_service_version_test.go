package sne

import "testing"

func TestRuntimePackageEffectiveServiceVersion(t *testing.T) {
	for _, test := range []struct {
		entry RuntimePackage
		want  string
	}{
		{RuntimePackage{ServiceVersion: "2.7.1", PackageID: "SNE-1.1.8-old"}, "2.7.1"},
		{RuntimePackage{PackageID: "SNE-1.1.8-e2b-nvfp4-m1-r5-ga-darwin-arm64"}, "1.1.8"},
		{RuntimePackage{PackageRoot: "/packages/SNE-2.7.1-12b-affine8-mtp-darwin-arm64"}, "2.7.1"},
		{RuntimePackage{PackageID: "unversioned-package"}, ""},
		{RuntimePackage{ServiceVersion: "dev", PackageID: "invalid"}, ""},
	} {
		if got := test.entry.EffectiveServiceVersion(); got != test.want {
			t.Fatalf("EffectiveServiceVersion(%+v)=%q want=%q", test.entry, got, test.want)
		}
	}
}
