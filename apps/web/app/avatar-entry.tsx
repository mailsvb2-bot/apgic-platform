"use client";

import { useEffect, useState } from "react";

// Preserve the canonical human HelpIntent entrance until a fully approved
// PersonaProvider really has usable personas. Never advertise a dead-end.
export function AvatarEntry() {
 const [available,setAvailable]=useState(false);
 useEffect(()=>{
  let alive=true;
  fetch("/v1/personas",{cache:"no-store",credentials:"same-origin"})
   .then(async response=>response.ok ? response.json() : {available:false})
   .then(data=>{if(alive)setAvailable(data.available===true&&Array.isArray(data.personas)&&data.personas.length>0)})
   .catch(()=>{if(alive)setAvailable(false)});
  return()=>{alive=false};
 },[]);
 return available
  ? <a className="secondary-action" href="/avatar">Поговорить с AI-аватаром</a>
  : <button className="secondary-action" type="button" disabled aria-describedby="avatar-status">
      Поговорить с аватаром
      <span className="coming-soon" id="avatar-status">скоро</span>
    </button>;
}
