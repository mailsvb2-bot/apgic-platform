import type {Metadata} from "next";
import {GrowthLanding} from "./visitor";
export const dynamic="force-dynamic";
export const metadata:Metadata={
 title:"Приглашение в APGIC",robots:{index:false,follow:false},referrer:"no-referrer",
};
export default async function Page({params}:{params:Promise<{token:string}>}){
 const {token}=await params;return <GrowthLanding token={token}/>;
}
