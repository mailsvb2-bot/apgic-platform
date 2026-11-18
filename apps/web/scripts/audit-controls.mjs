import fs from "node:fs";
import path from "node:path";
import ts from "typescript";

// Static complement to browser E2E: every application JSX button must be
// actionable, submit a real form or be explicitly disabled with a reason.
// This checks wiring presence, NOT correctness of a server-side business effect.
const root=path.resolve("app");
const failures=[];
let buttons=0,links=0,explainedDisabled=0;

function walk(folder){
 for(const entry of fs.readdirSync(folder,{withFileTypes:true})){
  if(entry.name.startsWith(".")||entry.name==="node_modules")continue;
  const filename=path.join(folder,entry.name);
  if(entry.isDirectory()){walk(filename);continue}
  if(!filename.endsWith(".tsx"))continue;
  const source=fs.readFileSync(filename,"utf8");
  const tree=ts.createSourceFile(filename,source,ts.ScriptTarget.Latest,true,ts.ScriptKind.TSX);
  function inspect(node,inForm=false){
   let childInForm=inForm;
   if(ts.isJsxElement(node)||ts.isJsxSelfClosingElement(node)){
    const element=ts.isJsxElement(node)?node.openingElement:node;
    const tag=element.tagName.getText(tree);
    childInForm=inForm||tag==="form";
    if(tag==="button"||tag==="a"){
     const attrs=element.attributes.properties;
     const has=(name)=>attrs.some(attr=>attr.name?.getText(tree)===name);
     const get=(name)=>attrs.find(attr=>attr.name?.getText(tree)===name)?.initializer?.getText(tree);
     const line=tree.getLineAndCharacterOfPosition(element.getStart(tree)).line+1;
     if(tag==="button"){
      buttons++;
      const type=get("type");
      const handler=has("onClick")||has("onPointerUp");
      const submit=childInForm&&(type===undefined||type==='"submit"')||has("form")&&type==='"submit"';
      const disabled=has("disabled")&&!handler&&!submit;
      if(disabled&&(has("aria-describedby")||has("title")))explainedDisabled++;
      else if(!handler&&!submit)failures.push(`${path.relative(root,filename)}:${line}: button has no handler/form submission or explained disabled state`);
     }else{
      links++;
      const href=get("href");
      if(href==='""'||href==="'#'"||href==='"#"'||href==='"javascript:void(0)"')
       failures.push(`${path.relative(root,filename)}:${line}: placeholder or empty href`);
     }
    }
   }
   ts.forEachChild(node,child=>inspect(child,childInForm));
  }
  inspect(tree);
 }
}
walk(root);
console.log(`APGIC JSX control wiring: ${buttons} buttons, ${links} links, ${explainedDisabled} explained unavailable controls`);
if(failures.length){failures.forEach(f=>console.error(f));process.exitCode=1}
