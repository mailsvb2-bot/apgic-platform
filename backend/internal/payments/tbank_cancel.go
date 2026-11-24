package payments

import (
 "bytes"
 "context"
 "encoding/json"
 "errors"
 "io"
 "net/http"
 "strings"
)

const tbankCancelEndpoint = "https://securepay.tinkoff.ru/v2/Cancel"

var ErrTBankCancel = errors.New("tbank cancel/refund execution unavailable")

// TBankCancelRequest is ONLY a PSP execution instruction. Its PaymentID and
// AmountMinor must originate from a durable, approved canonical refund
// request linked to the original captured attempt and provider. Never call
// from a browser supplied PaymentId or without the SQL one-shot refund claim.
type TBankCancelRequest struct {
 PaymentID string
 AmountMinor int64
}

type TBankCancelResult struct {
 PaymentID string
 Status string
}

// Cancel requests the official acquiring /v2/Cancel operation. It cannot
// conclude that funds were returned, update a refund, or revoke an Entitlement.
// A timeout/ambiguous response MUST be reconciled, not blindly retried.
// Existing TBankAcquiring maintains the signed bank/TLS transport owner.
func (a *TBankAcquiring) Cancel(ctx context.Context,request TBankCancelRequest)(TBankCancelResult,error){
 if a==nil||a.client==nil||ctx==nil||request.AmountMinor<=0||
  request.AmountMinor>10000000000||len(request.PaymentID)==0||
  len(request.PaymentID)>20{
  return TBankCancelResult{},ErrTBankCancel
 }
 for _,digit:=range request.PaymentID{
  if digit<'0'||digit>'9'{return TBankCancelResult{},ErrTBankCancel}
 }
 fields:=map[string]any{
  "TerminalKey":a.terminalKey,"PaymentId":request.PaymentID,
  "Amount":request.AmountMinor,
 }
 token,err:=TBankToken(fields,a.password)
 if err!=nil{return TBankCancelResult{},ErrTBankCancel}
 fields["Token"]=token
 payload,err:=json.Marshal(fields)
 if err!=nil{return TBankCancelResult{},ErrTBankCancel}
 req,err:=http.NewRequestWithContext(ctx,http.MethodPost,tbankCancelEndpoint,bytes.NewReader(payload))
 if err!=nil{return TBankCancelResult{},ErrTBankCancel}
 req.Header.Set("Content-Type","application/json")
 req.Header.Set("Accept","application/json")
 resp,err:=a.client.Do(req)
 if err!=nil{return TBankCancelResult{},ErrTBankCancel}
 defer resp.Body.Close()
 if resp.StatusCode!=http.StatusOK{return TBankCancelResult{},ErrTBankCancel}
 raw,err:=io.ReadAll(io.LimitReader(resp.Body,8193))
 if err!=nil||len(raw)>8192{return TBankCancelResult{},ErrTBankCancel}
 if !tbankUniqueRootFields(raw){
  return TBankCancelResult{},ErrTBankCancel
 }
 var result struct{
  Success bool `json:"Success"`
  ErrorCode string `json:"ErrorCode"`
  PaymentID json.Number `json:"PaymentId"`
  Status string `json:"Status"`
 }
 decoder:=json.NewDecoder(bytes.NewReader(raw))
 decoder.UseNumber()
 if decoder.Decode(&result)!=nil||!result.Success||result.ErrorCode!="0"||
  string(result.PaymentID)!=request.PaymentID||result.Status==""||
  len(result.Status)>48||strings.ContainsAny(result.Status,"\r\n"){
  return TBankCancelResult{},ErrTBankCancel
 }
 // A successful API execution is not a settled refund. REFUNDED must later
 // be confirmed independently and recorded through apgic_apply_refund_success.
 return TBankCancelResult{PaymentID:request.PaymentID,Status:result.Status},nil
}
