package storage_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"ovc/backend/internal/adapters/storage"
	"ovc/backend/internal/core"
	"ovc/backend/internal/teststorage"
)

func fetchPresigned(t *testing.T, address string) (int, []byte) {
	t.Helper()
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, address, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(core.ErrorKind(err))
	}
	body, readErr := io.ReadAll(response.Body)
	if err := response.Body.Close(); err != nil || readErr != nil {
		t.Fatal("the presigned read did not finish")
	}
	return response.StatusCode, body
}

func TestPresignedReadServesOneObjectForFiveMinutes(t *testing.T) {
	local := teststorage.New(t)
	object := "presign/" + core.NewRandomID()
	data := patterned(4096, 31)
	if err := local.Store.Put(context.Background(), object, data, "application/pdf"); err != nil {
		t.Fatal(err)
	}
	presigner := storage.NewPresigner(local.Settings, local.Settings.Endpoint)

	address, err := presigner.PresignRead(context.Background(), object)

	if err != nil {
		t.Fatal(core.ErrorKind(err))
	}
	parsed, err := url.Parse(address.Reveal())
	if err != nil || parsed.Scheme+"://"+parsed.Host != local.Settings.Endpoint || parsed.Query().Get("X-Amz-Expires") != "300" ||
		!strings.HasSuffix(parsed.Path, "/"+object) || parsed.Query().Get("X-Amz-SignedHeaders") != "host" {
		t.Fatal("the presigned read is not a five-minute address of the object on the given endpoint")
	}
	status, body := fetchPresigned(t, address.Reveal())
	if status != http.StatusOK || string(body) != string(data) {
		t.Fatalf("the presigned read answered %d with %d bytes", status, len(body))
	}
	if printed := fmt.Sprintf("%v %+v %#v %s", address, address, address, address); strings.Contains(printed, object) || strings.Contains(printed, "X-Amz") {
		t.Fatal("a printed presigned read revealed the address")
	}
}

func TestPresignedReadUsesTheGivenEndpoint(t *testing.T) {
	local := teststorage.New(t)
	presigner := storage.NewPresigner(local.Settings, "https://conferencing-storage.ovc.example")

	address, err := presigner.PresignRead(context.Background(), "presign/elsewhere")

	if err != nil {
		t.Fatal(core.ErrorKind(err))
	}
	if !strings.HasPrefix(address.Reveal(), "https://conferencing-storage.ovc.example/"+local.Settings.Bucket+"/presign/elsewhere?") {
		t.Fatal("the presigned read does not name the conferencing server's storage endpoint")
	}
}

func TestPresigningWithoutKeysFails(t *testing.T) {
	local := teststorage.New(t)
	presigner := storage.NewPresigner(core.StorageSettings{Backend: core.StorageBackendS3, Region: local.Settings.Region, Bucket: local.Settings.Bucket}, local.Settings.Endpoint)

	address, err := presigner.PresignRead(context.Background(), "presign/unsigned")

	if err == nil || !address.IsZero() {
		t.Fatal("presigning without keys produced an address")
	}
}
