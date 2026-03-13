//go:build kgwctl

/*
Copyright kgateway Authors.

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

package printer

import (
	"bytes"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	gwcommon "sigs.k8s.io/gwctl/pkg/common"

	common "github.com/kgateway-dev/kgateway/v2/pkg/kgwctl/common"
)

func TestTablePrinter_printDirectResponse(t *testing.T) {
	options := PrinterOptions{}
	p := &TablePrinter{PrinterOptions: options}
	out := &bytes.Buffer{}

	for _, ns := range testData(t)[common.DirectResponseGK] {
		err := p.printDirectResponse(ns, out)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		p.Flush(out)
	}

	wantOut := `
NAMESPACE  NAME               TYPE            AGE
ns-1       direct-response-1  DirectResponse  <unknown>
`

	got := gwcommon.MultiLine(out.String())
	want := gwcommon.MultiLine(strings.TrimPrefix(wantOut, "\n"))

	if diff := cmp.Diff(want, got, gwcommon.MultiLineTransformer); diff != "" {
		t.Fatalf("Unexpected diff:\n\ngot =\n\n%v\n\nwant =\n\n%v\n\ndiff (-want, +got) =\n\n%v", got, want, gwcommon.MultiLine(diff))
	}
}
