from __future__ import annotations
import importlib.util
import subprocess
import unittest
from pathlib import Path
from unittest.mock import patch

ROOT=Path(__file__).resolve().parents[2]
P=ROOT/"tools/r0_r3_dual_party_readback.py"
spec=importlib.util.spec_from_file_location("dual_party_readback",P)
assert spec and spec.loader
mod=importlib.util.module_from_spec(spec)
spec.loader.exec_module(mod)
B="00000000-0000-4000-8000-000000000901"

def ready(role):
 return {"booking_id":B,"role":role,"booking_state":"CONFIRMED",
         "client_ready_at":"2026-10-19T10:00:00Z",
         "specialist_ready_at":"2026-10-19T10:00:01Z"}

def result():
 return {"booking_id":B,"state":"COMPLETED","provider_instance_id":"provider-1",
         "completion_evidence_ref":"signed-provider-event-1"}

class DualPartyTests(unittest.TestCase):
 def test_independent_consistent_views_pass(self):
  self.assertEqual(mod.validate_parties(ready("CLIENT"),ready("SPECIALIST"),result(),result(),B),[])
 def test_fail_closed_for_invalid_roles_and_unproven_completion(self):
  cases=[
   (0,"role","SPECIALIST","CLIENT_AUTHORIZATION_NOT_PROVEN"),
   (1,"role","CLIENT","SPECIALIST_AUTHORIZATION_NOT_PROVEN"),
   (0,"client_ready_at",None,"CLIENT_BOTH_PARTICIPANTS_NOT_READY"),
   (1,"specialist_ready_at",None,"SPECIALIST_BOTH_PARTICIPANTS_NOT_READY"),
   (2,"state","JOINED","CLIENT_PROVIDER_COMPLETION_NOT_PROVEN"),
   (3,"completion_evidence_ref","","SPECIALIST_PROVIDER_EVIDENCE_MISSING"),
   (3,"provider_instance_id","other-provider","CROSS_ROLE_PROVIDER_RESULT_MISMATCH"),
  ]
  for index,key,value,wanted in cases:
   with self.subTest(wanted=wanted):
    views=[ready("CLIENT"),ready("SPECIALIST"),result(),result()]
    views[index][key]=value
    self.assertIn(wanted,mod.validate_parties(*views,B))
 def test_stranger_access_must_be_denied_with_404(self):
  class FakeResponse:
   def __enter__(self): return self
   def __exit__(self,*args): pass
  class FakeOpener:
   def __init__(self,status): self.status=status
   def open(self,request,timeout):
    import urllib.error
    if self.status==404: raise urllib.error.HTTPError(request.full_url,404,"Not Found",None,None)
    return FakeResponse()
  for status,allowed in [(404,True),(200,False),(403,False)]:
   with self.subTest(status=status):
    with patch.object(mod.base.urllib.request,"build_opener",return_value=FakeOpener(status)):
     self.assertEqual(mod.denied_for_stranger("http://127.0.0.1:43111",B,"sid=unrelated"),allowed)
 def test_stranger_denial_covers_booking_and_both_consultation_projections(self):
  import urllib.error
  seen=[]
  class FakeOpener:
   def open(self,request,timeout):
    seen.append(request.full_url)
    raise urllib.error.HTTPError(request.full_url,404,"Not Found",None,None)
  with patch.object(mod.base.urllib.request,"build_opener",return_value=FakeOpener()):
   self.assertTrue(mod.denied_for_stranger("http://127.0.0.1:43111",B,"sid=unrelated"))
  self.assertEqual([url.split("43111",1)[1] for url in seen],[
   f"/v1/bookings/{B}",f"/v1/consultations/{B}/ready",f"/v1/consultations/{B}/result"])
 def test_stranger_booking_leak_fails_closed_before_consultation_checks(self):
  import urllib.error
  seen=[]
  class FakeResponse:
   def __enter__(self): return self
   def __exit__(self,*args): pass
  class FakeOpener:
   def open(self,request,timeout):
    seen.append(request.full_url)
    if "/v1/bookings/" in request.full_url: return FakeResponse()
    raise urllib.error.HTTPError(request.full_url,404,"Not Found",None,None)
  with patch.object(mod.base.urllib.request,"build_opener",return_value=FakeOpener()):
   self.assertFalse(mod.denied_for_stranger("http://127.0.0.1:43111",B,"sid=unrelated"))
  self.assertEqual(len(seen),1)
 def test_staging_wrapper_guards_host_and_requires_three_sessions(self):
  script=ROOT/"deploy/staging/prove-r0-r3-dual-party.sh"
  subprocess.run(["bash","-n",str(script)],check=True)
  text=script.read_text()
  for value in ("assert-authorized-host.sh","check-staging-runtime.sh",
   "APGIC_R0_R3_ACCEPTANCE_SESSION_COOKIE","APGIC_R0_R3_SPECIALIST_SESSION_COOKIE",
   "APGIC_R0_R3_STRANGER_SESSION_COOKIE"):
   self.assertIn(value,text)
if __name__=="__main__":
 unittest.main()
