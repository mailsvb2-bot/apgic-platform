package academy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestYandexDiskClientRejectsInvalidRequests(t *testing.T) {
	client := YandexDiskClient{}
	if _,_,err:=client.ListFiles(context.Background(),"secret","https://evil.example",0,10);err==nil {
		t.Fatal("external path accepted")
	}
	if _,_,err:=client.ListFiles(context.Background(),"secret","disk:/",0,1001);err==nil {
		t.Fatal("unbounded limit accepted")
	}
	if _,err:=client.TemporaryDownload(context.Background(),"secret","https://evil.example");err==nil {
		t.Fatal("external download accepted")
	}
}

func TestYandexDiskClientDoesNotExposeOAuthTokenInErrors(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
		if r.Header.Get("Authorization")!="OAuth secret-token"{t.Error("missing OAuth header")}
		w.WriteHeader(http.StatusForbidden)
		_,_=w.Write([]byte("secret-token"))
	}))
	defer s.Close()
	client:=YandexDiskClient{Client:s.Client()}
	err:=client.do(context.Background(),"secret-token",s.URL,&struct{}{})
	if err==nil||err.Error()!="cloud provider response 403" {
		t.Fatalf("unexpected error: %v",err)
	}
}
