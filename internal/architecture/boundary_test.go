// Package architecture holds the static import-boundary checks that keep each layer's
// dependencies pointing the way the architecture says they do. It has no non-test source: the
// checks exist only to run under `make check`.
package architecture

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const modulePath = "github.com/Stewz00/wattfeder"

func TestHouseholdDomainStaysFreeOfTransportAndStorageImports(t *testing.T) {
	got, err := disallowedImports("../household", nil)
	if err != nil {
		t.Fatalf("disallowedImports() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("internal/household imports outside the standard library: %v", got)
	}
}

// The runtime is written against its own TelemetrySource and CommandSink contracts, and the
// simulator satisfies them structurally, without either package naming the other. Nothing in
// the compiler preserves that direction, so an import of any adapter into the runtime — the
// simulator above all — has to fail here instead.
func TestApplicationRuntimeNeverImportsAnAdapter(t *testing.T) {
	// The runtime's own integration tests run against real SQLite rather than a fake
	// repository, which is why the storage adapter is inside the allowlist and the telemetry
	// adapter is not.
	allowed := []string{
		modulePath + "/internal/household",
		modulePath + "/internal/persistence",
	}

	got, err := disallowedImports("../application", allowed)
	if err != nil {
		t.Fatalf("disallowedImports() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("internal/application imports outside the domain and persistence contracts: %v", got)
	}
}

func TestDisallowedImportsFindsNothingOutsideTheAllowlist(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, dir, "clean.go", `package example

import (
	"fmt"
	"time"
)

var _ = fmt.Sprintf
var _ time.Duration
`)

	got, err := disallowedImports(dir, nil)
	if err != nil {
		t.Fatalf("disallowedImports() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("disallowedImports() = %v, want none", got)
	}
}

func TestDisallowedImportsFlagsAnImportOutsideTheAllowlist(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, dir, "violation.go", `package example

import (
	"fmt"
	"github.com/Stewz00/wattfeder/internal/persistence"
)

var _ = fmt.Sprintf
var _ persistence.Repository
`)

	got, err := disallowedImports(dir, nil)
	if err != nil {
		t.Fatalf("disallowedImports() error = %v", err)
	}
	if len(got) != 1 || got[0] != modulePath+"/internal/persistence" {
		t.Errorf("disallowedImports() = %v, want exactly the persistence import", got)
	}
}

func TestDisallowedImportsHonorsAnExplicitAllowedPrefix(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, dir, "allowed.go", `package example

import "github.com/Stewz00/wattfeder/internal/household"

var _ household.EventID
`)

	got, err := disallowedImports(dir, []string{modulePath + "/internal/household"})
	if err != nil {
		t.Fatalf("disallowedImports() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("disallowedImports() = %v, want none (household is explicitly allowed)", got)
	}
}

// An allowed prefix covers the packages nested under it, which is what lets one entry for the
// persistence contracts also admit the SQLite adapter the integration tests need.
func TestDisallowedImportsAllowedPrefixCoversNestedPackages(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, dir, "nested.go", `package example

import "github.com/Stewz00/wattfeder/internal/persistence/sqlite"

var _ = sqlite.Open
`)

	got, err := disallowedImports(dir, []string{modulePath + "/internal/persistence"})
	if err != nil {
		t.Fatalf("disallowedImports() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("disallowedImports() = %v, want none (persistence/sqlite sits under an allowed prefix)", got)
	}
}

// A prefix must match whole path segments, so allowing one package never quietly allows a
// differently named sibling that happens to share its opening characters.
func TestDisallowedImportsAllowedPrefixDoesNotMatchAPartialSegment(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, dir, "sibling.go", `package example

import "github.com/Stewz00/wattfeder/internal/householdmocks"

var _ = householdmocks.Anything
`)

	got, err := disallowedImports(dir, []string{modulePath + "/internal/household"})
	if err != nil {
		t.Fatalf("disallowedImports() error = %v", err)
	}
	if len(got) != 1 {
		t.Errorf("disallowedImports() = %v, want the householdmocks import flagged", got)
	}
}

func TestDisallowedImportsCoversEveryFileInTheDirectoryIncludingTests(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, dir, "main.go", `package example

var _ = 1
`)
	writeGoFile(t, dir, "main_test.go", `package example

import "github.com/Stewz00/wattfeder/internal/persistence/sqlite"

var _ = sqlite.Open
`)

	got, err := disallowedImports(dir, nil)
	if err != nil {
		t.Fatalf("disallowedImports() error = %v", err)
	}
	if len(got) != 1 || got[0] != modulePath+"/internal/persistence/sqlite" {
		t.Errorf("disallowedImports() = %v, want exactly the sqlite import from the test file", got)
	}
}

func writeGoFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// disallowedImports parses every .go file directly under dir and returns the sorted, deduplicated
// import paths that are neither part of the standard library nor listed in allowedPrefixes. An
// import is treated as standard library when its first path segment contains no dot, which is
// the same heuristic Go tooling uses to tell a module path from a standard import path.
func disallowedImports(dir string, allowedPrefixes []string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read directory: %w", err)
	}

	violations := make(map[string]struct{})
	fileSet := token.NewFileSet()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		file, err := parser.ParseFile(fileSet, path, nil, parser.ImportsOnly)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}

		for _, spec := range file.Imports {
			importPath := strings.Trim(spec.Path.Value, `"`)
			if isStandardLibrary(importPath) || hasAnyPrefix(importPath, allowedPrefixes) {
				continue
			}
			violations[importPath] = struct{}{}
		}
	}

	result := make([]string, 0, len(violations))
	for importPath := range violations {
		result = append(result, importPath)
	}
	slices.Sort(result)
	return result, nil
}

func isStandardLibrary(importPath string) bool {
	firstSegment, _, _ := strings.Cut(importPath, "/")
	return !strings.Contains(firstSegment, ".")
}

func hasAnyPrefix(importPath string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if importPath == prefix || strings.HasPrefix(importPath, prefix+"/") {
			return true
		}
	}
	return false
}
