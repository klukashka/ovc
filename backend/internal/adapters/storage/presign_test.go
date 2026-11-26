package storage_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

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

func TestPresignedDownloadLastsTheGivenLifetime(t *testing.T) {
	local := teststorage.New(t)
	object := "presign/" + core.NewRandomID()
	data := patterned(2048, 17)
	if err := local.Store.Put(context.Background(), object, data, "application/zip"); err != nil {
		t.Fatal(err)
	}
	presigner := storage.NewPresigner(local.Settings, local.Settings.Endpoint)

	address, err := presigner.PresignDownload(context.Background(), object, "application/zip", `attachment; filename="export.zip"`, 90*time.Second)

	if err != nil {
		t.Fatal(core.ErrorKind(err))
	}
	parsed, err := url.Parse(address.Reveal())
	if err != nil || parsed.Query().Get("X-Amz-Expires") != "90" || parsed.Query().Get("X-Amz-SignedHeaders") != "host" {
		t.Fatal("the presigned download does not last the given ninety seconds")
	}
	status, body := fetchPresigned(t, address.Reveal())
	if status != http.StatusOK || string(body) != string(data) {
		t.Fatalf("the presigned download answered %d with %d bytes", status, len(body))
	}
}

func TestPresignedDownloadRefusesALifetimeOutsideTheReadLifetime(t *testing.T) {
	local := teststorage.New(t)
	presigner := storage.NewPresigner(local.Settings, local.Settings.Endpoint)

	none, noneErr := presigner.PresignDownload(context.Background(), "presign/none", "application/zip", "attachment", 0)
	long, longErr := presigner.PresignDownload(context.Background(), "presign/long", "application/zip", "attachment", storage.PresignedReadLifetime+time.Second)

	if !errors.Is(noneErr, storage.ErrPresignLifetime) || !errors.Is(longErr, storage.ErrPresignLifetime) || !none.IsZero() || !long.IsZero() {
		t.Fatal("a presigned download outside the read lifetime was produced")
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
