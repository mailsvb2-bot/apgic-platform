// Authorization failures are not optional Academy metadata failures.
// Keep one explicit policy for all native Academy requests.
export class AcademyAccessError extends Error {
 readonly status:number;
 constructor(status:number,message:string){
  super(message);
  this.name="AcademyAccessError";
  this.status=status;
 }
}
export function academySessionRejected(error:unknown):boolean {
 return error instanceof AcademyAccessError && error.status===401;
}
export async function academyOptional<T>(request:Promise<T>,fallback:T):Promise<T>{
 try{return await request}
 catch(error){
  // Failing open here could expose private enrollments from a previous identity.
  if(academySessionRejected(error))throw error;
  return fallback;
 }
}
