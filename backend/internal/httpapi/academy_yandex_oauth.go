package httpapi

import (
 "crypto/hmac"
 "crypto/rand"
 "crypto/sha256"
 "encoding/base64"
 "encoding/json"
 "errors"
 "io"
 "net/http"
 "net/url"
 "os"
 "strings"
 "time"

 "github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

type academyCloudCredentialWriter interface { AcademyStoreCloudCredential(actor,provider,token string) error }
type cloudOAuthState struct {
 Actor string `json:"actor"`
 Verifier string `json:"verifier"`
 Nonce string `json:"nonce"`
 Expires int64 `json:"expires"`
}
func cloudRandom(n int)(string,error){
 b:=make([]byte,n)
 if _,err:=rand.Read(b);err!=nil{return "",err}
 return base64.RawURLEncoding.EncodeToString(b),nil
}
func cloudStateKey()([]byte,error){
 s:=os.Getenv("APGIC_ACADEMY_OAUTH_STATE_KEY")
 key,err:=base64.StdEncoding.DecodeString(s)
 if err!=nil||len(key)<32{return nil,errors.New("OAuth state key is unavailable")}
 return key,nil
}
func cloudSeal(state cloudOAuthState,key []byte)(string,error){
 raw,err:=json.Marshal(state)
 if err!=nil{return "",err}
 mac:=hmac.New(sha256.New,key)
 mac.Write(raw)
 return base64.RawURLEncoding.EncodeToString(raw)+"."+base64.RawURLEncoding.EncodeToString(mac.Sum(nil)),nil
}
func cloudOpen(value string,key []byte)(cloudOAuthState,error){
 var state cloudOAuthState
 parts:=strings.Split(value,".")
 if len(parts)!=2{return state,academy.ErrDenied}
 raw,err:=base64.RawURLEncoding.DecodeString(parts[0])
 if err!=nil||len(raw)>2048{return state,academy.ErrDenied}
 signature,err:=base64.RawURLEncoding.DecodeString(parts[1])
 if err!=nil{return state,academy.ErrDenied}
 mac:=hmac.New(sha256.New,key)
 mac.Write(raw)
 if !hmac.Equal(signature,mac.Sum(nil)){return state,academy.ErrDenied}
 if json.Unmarshal(raw,&state)!=nil||state.Actor==""||state.Verifier==""||state.Nonce==""||state.Expires<time.Now().Unix(){return state,academy.ErrDenied}
 return state,nil
}
func academyYandexOAuthConfig()(string,string,string,error){
 id:=os.Getenv("APGIC_YANDEX_OAUTH_CLIENT_ID")
 secret:=os.Getenv("APGIC_YANDEX_OAUTH_CLIENT_SECRET")
 redirect:=os.Getenv("APGIC_YANDEX_OAUTH_REDIRECT_URI")
 u,err:=url.Parse(redirect)
 if id==""||secret==""||err!=nil||u.Scheme!="https"||u.Host==""||u.User!=nil||u.Fragment!=""||u.Path!="/v1/academy/author/cloud/yandex/callback"||u.RawQuery!="" {
  return "","","",academy.ErrUnavailable
 }
 return id,secret,redirect,nil
}
func registerAcademyYandexOAuth(mux *http.ServeMux,store academyStore,sessions *clientSessionManager,sessionErr error){
 const cookieName="apgic_academy_yandex_oauth"
 mux.HandleFunc("GET /v1/academy/author/cloud/yandex/connect",func(w http.ResponseWriter,r *http.Request){
  actor,ok:=academyIdentity(w,r,sessions,sessionErr);if !ok{return}
  id,_,redirect,err:=academyYandexOAuthConfig();if err!=nil{academyHTTPError(w,r,err);return}
  key,err:=cloudStateKey();if err!=nil{academyHTTPError(w,r,academy.ErrUnavailable);return}
  verifier,err:=cloudRandom(32);if err!=nil{academyHTTPError(w,r,academy.ErrUnavailable);return}
  nonce,err:=cloudRandom(24);if err!=nil{academyHTTPError(w,r,academy.ErrUnavailable);return}
  value,err:=cloudSeal(cloudOAuthState{Actor:actor,Verifier:verifier,Nonce:nonce,Expires:time.Now().Add(10*time.Minute).Unix()},key)
  if err!=nil{academyHTTPError(w,r,academy.ErrUnavailable);return}
  http.SetCookie(w,&http.Cookie{Name:cookieName,Value:value,Path:"/v1/academy/author/cloud/yandex",MaxAge:600,Secure:true,HttpOnly:true,SameSite:http.SameSiteLaxMode})
  challenge:=sha256.Sum256([]byte(verifier))
  q:=url.Values{"response_type":{"code"},"client_id":{id},"redirect_uri":{redirect},"state":{nonce},"code_challenge":{base64.RawURLEncoding.EncodeToString(challenge[:])},"code_challenge_method":{"S256"}}
  w.Header().Set("Cache-Control","no-store")
  http.Redirect(w,r,"https://oauth.yandex.ru/authorize?"+q.Encode(),http.StatusFound)
 })
 mux.HandleFunc("GET /v1/academy/author/cloud/yandex/callback",func(w http.ResponseWriter,r *http.Request){
  w.Header().Set("Cache-Control","no-store")
  cookie,err:=r.Cookie(cookieName)
  http.SetCookie(w,&http.Cookie{Name:cookieName,Path:"/v1/academy/author/cloud/yandex",Value:"",MaxAge:-1,Secure:true,HttpOnly:true,SameSite:http.SameSiteLaxMode})
  if err!=nil{academyHTTPError(w,r,academy.ErrDenied);return}
  key,err:=cloudStateKey();if err!=nil{academyHTTPError(w,r,academy.ErrUnavailable);return}
  state,err:=cloudOpen(cookie.Value,key)
  if err!=nil||r.URL.Query().Get("state")!=state.Nonce||r.URL.Query().Get("code")==""||len(r.URL.Query().Get("code"))>2048 {
   academyHTTPError(w,r,academy.ErrDenied);return
  }
  writer,ok:=store.(academyCloudCredentialWriter);if !ok{academyHTTPError(w,r,academy.ErrUnavailable);return}
  id,secret,_,err:=academyYandexOAuthConfig();if err!=nil{academyHTTPError(w,r,err);return}
  form:=url.Values{"grant_type":{"authorization_code"},"code":{r.URL.Query().Get("code")},"code_verifier":{state.Verifier}}
  req,err:=http.NewRequestWithContext(r.Context(),http.MethodPost,"https://oauth.yandex.ru/token",strings.NewReader(form.Encode()))
  if err!=nil{academyHTTPError(w,r,academy.ErrUnavailable);return}
  req.Header.Set("Content-Type","application/x-www-form-urlencoded")
  req.SetBasicAuth(id,secret)
  client:=&http.Client{Timeout:12*time.Second,CheckRedirect:func(_ *http.Request,_ []*http.Request)error{return http.ErrUseLastResponse}}
  response,err:=client.Do(req);if err!=nil{academyHTTPError(w,r,academy.ErrUnavailable);return}
  defer response.Body.Close()
  if response.StatusCode!=http.StatusOK{academyHTTPError(w,r,academy.ErrDenied);return}
  var token struct{AccessToken string `json:"access_token"`}
  decoder:=json.NewDecoder(io.LimitReader(response.Body,16384))
  if decoder.Decode(&token)!=nil||token.AccessToken==""||len(token.AccessToken)>8192{academyHTTPError(w,r,academy.ErrUnavailable);return}
  if writer.AcademyStoreCloudCredential(state.Actor,"YANDEX_DISK",token.AccessToken)!=nil{academyHTTPError(w,r,academy.ErrUnavailable);return}
  http.Redirect(w,r,"/academy?cloud_connected=yandex",http.StatusSeeOther)
 })
}
