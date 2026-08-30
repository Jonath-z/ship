package httpx

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
)

func TestEveryOpenAPIOperationDeclaresPermission(t *testing.T) {
	scanner := bufio.NewScanner(bytes.NewReader(OpenAPISpec))
	operations := 0
	permissions := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "operationId:") {
			operations++
		}
		if strings.HasPrefix(line, "x-ship-permission:") {
			permissions++
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if operations == 0 || operations != permissions {
		t.Fatalf("OpenAPI has %d operations and %d permission declarations", operations, permissions)
	}
}
