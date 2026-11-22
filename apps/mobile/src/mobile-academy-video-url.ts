const idPattern=/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

// The media source is always the protected first-party APGIC gateway. No
// provider paths, cloud URLs, query credentials, or arbitrary hosts accepted.
export function nativeAcademyVideoPath(enrollmentId:string,lessonId:string,mediaId:string):string|null{
 if(![enrollmentId,lessonId,mediaId].every(id=>idPattern.test(id)))return null;
 return `/v1/academy/enrollments/${enrollmentId}/lessons/${lessonId}/videos/${mediaId}/stream`;
}
