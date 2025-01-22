// Copyright 2024 go-dataspace
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package shared

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/alecthomas/chroma/v2/quick"
	"github.com/fatih/color"
	"github.com/go-dataspace/run-dsp/internal/ui"
	dspcontrol "github.com/go-dataspace/run-dsrpc/gen/go/dsp/v1alpha2"
	"github.com/spf13/viper"
)

// PrintCatalogue prints out a catalogue, either as a table or as JSON.
func PrintCatalogue(catalogue []*dspcontrol.Dataset, printJSON bool) error {
	if printJSON {
		return pprintJSON(catalogue)
	}

	for _, ds := range catalogue {
		err := PrintDataset(ds, printJSON)
		if err != nil {
			return err
		}
		fmt.Println("") //nolint:forbidigo
	}
	return nil
}

func pprintJSON[T any](o T) error {
	b, err := json.Marshal(o)
	if err != nil {
		return fmt.Errorf("could not marshal datasets: %w", err)
	}
	var buf bytes.Buffer
	err = json.Indent(&buf, b, "", "  ")
	if err != nil {
		return fmt.Errorf("could not indent JSON: %w", err)
	}
	if viper.GetBool(NoColor) {
		ui.Print(buf.String())
		return nil
	}
	return quick.Highlight(os.Stdout, buf.String(), "json", "terminal256", "catppuccin-mocha")
}

// PrintDataset prints out a dataset, either as a table or as JSON.
func PrintDataset(ds *dspcontrol.Dataset, printJSON bool) error {
	if printJSON {
		return pprintJSON(ds)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', 0)
	fmt.Fprintf(w, "%s\t%s\n", color.New(color.Bold).Sprint("ID"), ds.GetId())
	fmt.Fprintf(w, "%s\t%s\n", color.New(color.Bold).Sprint("Title"), ds.GetTitle())
	fmt.Fprintf(w, "%s\t%s\n", color.New(color.Bold).Sprint("Access Methods"), ds.GetAccessMethods())
	for _, ml := range ds.GetDescription() {
		fmt.Fprintf(w, "%s\t%s\n", color.New(color.Bold).Sprintf("Description (%s)", ml.GetLanguage()), ml.GetValue())
	}
	fmt.Fprintf(w, "%s\t%s\n", color.New(color.Bold).Sprint("Keywords"), strings.Join(ds.GetKeywords(), ", "))
	fmt.Fprintf(w, "%s\t%s\n", color.New(color.Bold).Sprint("Creator"), ds.GetCreator())
	fmt.Fprintf(w, "%s\t%s\n", color.New(color.Bold).Sprint("Issued"), ds.GetIssued().AsTime().Format(time.RFC3339))
	fmt.Fprintf(w, "%s\t%s\n", color.New(color.Bold).Sprint("Modified"), ds.GetModified().AsTime().Format(time.RFC3339))
	for k, v := range ds.GetMetadata() {
		fmt.Fprintf(w, "%s\t%s\n", color.New(color.Bold).Sprintf("Metadata %s\n", k), v)
	}
	fmt.Fprintf(w, "%s\t%s\n", color.New(color.Bold).Sprint("License"), ds.GetLicense())
	fmt.Fprintf(w, "%s\t%s\n", color.New(color.Bold).Sprint("AccessRights"), ds.GetAccessRights())
	fmt.Fprintf(w, "%s\t%s\n", color.New(color.Bold).Sprint("Rights"), ds.GetRights())
	fmt.Fprintf(w, "%s\t%d\n", color.New(color.Bold).Sprint("ByteSize"), ds.GetByteSize())
	fmt.Fprintf(w, "%s\t%s\n", color.New(color.Bold).Sprint("MediaType"), ds.GetMediaType())
	fmt.Fprintf(w, "%s\t%s\n", color.New(color.Bold).Sprint("Format"), ds.GetFormat())
	fmt.Fprintf(w, "%s\t%s\n", color.New(color.Bold).Sprint("CompressFormat"), ds.GetCompressFormat())
	fmt.Fprintf(w, "%s\t%s\n", color.New(color.Bold).Sprint("PackageFormat"), ds.GetPackageFormat())
	if ck := ds.GetChecksum(); ck != nil {
		fmt.Fprintf(w, "%s\t%s\n", color.New(color.Bold).Sprint("Checksum: Algorithm"), ck.GetAlgorithm())
		fmt.Fprintf(w, "%s\t%s\n", color.New(color.Bold).Sprint("Checksum: Value"), ck.GetValue())
	} else {
		fmt.Fprintf(w, "%s\t%s\n", color.New(color.Bold).Sprint("Checksum: Algorithm"), "")
		fmt.Fprintf(w, "%s\t%s\n", color.New(color.Bold).Sprint("Checksum: Value"), "")
	}
	w.Flush()
	return nil
}
