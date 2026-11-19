package api

import (
	"net/http"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/cacack/my-family/internal/config"
	"github.com/cacack/my-family/internal/query"
	"github.com/cacack/my-family/internal/repository"
	"github.com/cacack/my-family/internal/repository/memory"
)

// adrPath is ADR-008, whose read-surface table assigns every operation that
// returns tree data to a choke point.
const adrPath = "../../docs/adr/008-privacy-model.md"

// adrRow matches the operationId cell that opens a row of the table.
var adrRow = regexp.MustCompile(`(?m)^\| ([a-z][A-Za-z]+) \|`)

// Every operationId in openapi.yaml has a row in ADR-008's read-surface
// table, so a new endpoint cannot go unclassified (ADR-008 §7). Editor- and
// owner-only writes are covered by the table's row for every write, since
// nothing is redacted for those roles; a write a viewer or visitor may call
// needs its own row. A row naming an operation the spec no longer has fails
// too, so the table does not go stale.
func TestPrivacy_EveryOperationHasAReadSurfaceRow(t *testing.T) {
	adr, err := os.ReadFile(adrPath)
	if err != nil {
		t.Fatalf("read ADR-008: %v", err)
	}
	rows := map[string]bool{}
	for _, m := range adrRow.FindAllStringSubmatch(string(adr), -1) {
		rows[m[1]] = true
	}
	doc, err := openapi3.NewLoader().LoadFromData(openapiSpec)
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}
	inSpec := map[string]bool{}
	for path, item := range doc.Paths.Map() {
		for method, op := range item.Operations() {
			inSpec[op.OperationID] = true
			role, _ := op.Extensions["x-min-role"].(string)
			editorWrite := method != http.MethodGet && repository.Role(role).Includes(repository.RoleEditor)
			if !rows[op.OperationID] && !editorWrite {
				t.Errorf("%s %s (%s, x-min-role %s) has no row in ADR-008's read-surface table", method, path, op.OperationID, role)
			}
		}
	}
	for id := range rows {
		if !inSpec[id] {
			t.Errorf("ADR-008's read-surface table has a row for %s, which openapi.yaml does not define", id)
		}
	}
}

// Every query service the server builds reads the read model through the
// privacy decorator, and so do the handlers: a service wired to the raw
// store would show everyone everything (ADR-008 §5). Only the command
// handler, which writes, holds the raw store.
func TestPrivacy_QueryServicesReadThroughTheDecorator(t *testing.T) {
	events := memory.NewEventStore()
	server := NewServer(&config.Config{AuthMode: config.AuthModeOpen}, events, memory.NewReadModelStore(),
		memory.NewSnapshotStore(events), nil, WithBranchStore(memory.NewBranchStore()))
	decorator := reflect.TypeOf((*query.PrivacyStore)(nil))
	storeType := reflect.TypeOf((*repository.ReadModelStore)(nil)).Elem()

	if got := reflect.ValueOf(server.readStore).Type(); got != decorator {
		t.Errorf("server.readStore is %v, want %v", got, decorator)
	}
	queryPkg := decorator.Elem().PkgPath()
	seen := map[uintptr]bool{}
	services := 0
	var walk func(path string, v reflect.Value)
	walk = func(path string, v reflect.Value) {
		switch v.Kind() {
		case reflect.Pointer:
			if v.IsNil() || seen[v.Pointer()] || v.Elem().Kind() != reflect.Struct || v.Type() == decorator {
				return
			}
			seen[v.Pointer()] = true
			if v.Elem().Type().PkgPath() == queryPkg {
				services++
			}
			walk(path, v.Elem())
		case reflect.Struct:
			for i := range v.NumField() {
				field := v.Type().Field(i)
				fv := v.Field(i)
				if field.Type == storeType && !fv.IsNil() {
					if got := fv.Elem().Type(); got != decorator {
						t.Errorf("%s.%s reads the read model through %v, not the privacy decorator", path, field.Name, got)
					}
					continue
				}
				if field.Type.Kind() == reflect.Pointer && field.Type.Elem().PkgPath() == queryPkg {
					walk(path+"."+field.Name, fv)
				}
			}
		}
	}
	for i := range reflect.ValueOf(server).Elem().NumField() {
		field := reflect.TypeOf(server).Elem().Field(i)
		if strings.HasSuffix(field.Name, "Service") {
			walk("Server."+field.Name, reflect.ValueOf(server).Elem().Field(i))
		}
	}
	if services < 20 {
		t.Errorf("checked %d query services; the walk no longer reaches them", services)
	}
	if server.rawReadStore == nil || reflect.TypeOf(server.rawReadStore) == decorator {
		t.Error("the server keeps no raw store for its writes")
	}
}
