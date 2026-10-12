import { PublicSpecialistProfile } from "./public-profile";

export const dynamic = "force-dynamic";
export default async function SpecialistPublicPage({params}:{params:Promise<{id:string}>}){
 const {id}=await params;
 return <PublicSpecialistProfile id={id}/>;
}
