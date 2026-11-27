const idPattern=/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

// A manifest request must be bound to the same canonical UUID identifiers as
// its protected byte-stream route. Never encode arbitrary navigation input into
// an authenticated Academy request.
export function nativeAcademyManifestPath(enrollmentId:string,lessonId:string,scope:"learner"|"author"="learner"):string|null{
 if(!idPattern.test(enrollmentId)||!idPattern.test(lessonId))return null;
 return `/v1/academy/${scope==="author"?"author/versions":"enrollments"}/${enrollmentId}/lessons/${lessonId}/videos`;
}

// The media source is always the protected first-party APGIC gateway. No
// provider paths, cloud URLs, query credentials, or arbitrary hosts accepted.
export function nativeAcademyVideoPath(enrollmentId:string,lessonId:string,mediaId:string,scope:"learner"|"author"="learner"):string|null{
 if(![enrollmentId,lessonId,mediaId].every(id=>idPattern.test(id)))return null;
 return `${nativeAcademyManifestPath(enrollmentId,lessonId,scope)}/${mediaId}/stream`;
}

// Reject secret-bearing nested metadata too: checking only direct keys allows
// {provider:{accessToken:...}} to pass into the native authorization boundary.
// Bound recursion so malformed provider responses cannot exhaust the client.
function hasForbiddenManifestMetadata(value:unknown,depth=0,budget={remaining:512}):boolean{
 if(--budget.remaining<0||depth>8)return true;
 if(value===null||typeof value!=="object")return false;
 for(const [key,child] of Object.entries(value)){
  if(/(?:url|uri|href|token|authorization|cookie|secret|credential)/i.test(key)||
     hasForbiddenManifestMetadata(child,depth+1,budget))return true;
 }
 return false;
}

export type NativeAcademyVideo={id:string;title:string;ordinal:number};
// Treat the server manifest as an authorization projection, not a media URL.
// Do not accept provider URLs, duplicate IDs or unbounded/ambiguous payloads.
export function nativeAcademyValidatedVideos(
 payload:unknown,enrollmentId:string,lessonId:string,
):NativeAcademyVideo[]|null{
 if(!payload||typeof payload!=="object"||Array.isArray(payload)||!Array.isArray((payload as {videos?:unknown}).videos))return null;
 // Fail closed if the manifest envelope itself leaks upstream URLs or secrets.
 // Checking only individual video entries misses provider metadata at the root.
 if(hasForbiddenManifestMetadata(Object.fromEntries(Object.entries(payload).filter(([key])=>key!=="videos"))))return null;
 const raw=(payload as {videos:unknown[]}).videos;
 if(raw.length===0||raw.length>100)return null;
 const seen=new Set<string>();
 const ordinals=new Set<number>();
 const videos:NativeAcademyVideo[]=[];
 for(const entry of raw){
  if(!entry||typeof entry!=="object")return null;
  const video=entry as Partial<NativeAcademyVideo>;
  // A manifest is metadata, never a carrier for upstream credentials or
  // provider addresses. Reject accidental server-side leakage outright.
  if(hasForbiddenManifestMetadata(entry))return null;
  if(typeof video.id!=="string"||!nativeAcademyVideoPath(enrollmentId,lessonId,video.id)||
   seen.has(video.id.toLowerCase())||typeof video.title!=="string"||
   video.title.trim().length===0||video.title.length>300||
   /[\u0000-\u001f\u007f\u202a-\u202e\u2066-\u2069]/.test(video.title)||
   typeof video.ordinal!=="number"||!Number.isSafeInteger(video.ordinal)||video.ordinal<0||ordinals.has(video.ordinal))return null;
  seen.add(video.id.toLowerCase());
  ordinals.add(video.ordinal);
  videos.push({id:video.id,title:video.title,ordinal:video.ordinal});
 }
 return videos;
}
