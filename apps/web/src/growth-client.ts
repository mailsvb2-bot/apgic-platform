// Growth URLs and prefills deliberately contain no private consultation content.
export const growthTopics: Record<string,string> = {
  anxiety: "Меня беспокоит тревога и стресс. Хочу найти специалиста.",
  sleep: "Мне сложно со сном. Хочу найти специалиста.",
  relationships: "Хочу обсудить сложности в отношениях.",
  career: "Хочу разобраться в карьерных задачах.",
};
export type GrowthTouch = {
  link_id:string;source:"SPECIALIST_SOURCED";offer_id:string;
  topic_id:string;specialist_name:string;idempotent:boolean;
};
export function growthTarget(topic:string):string {
  return Object.hasOwn(growthTopics,topic) ? "/?topic="+encodeURIComponent(topic)+"#start" : "/#start";
}
export function growthURL(path:string,origin:string):string {
  if(!/^\/g\/g1\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$/.test(path))throw Error("GROWTH_LINK_INVALID");
  const o=new URL(origin);
  if(o.protocol!=="https:" && !["localhost","127.0.0.1","[::1]"].includes(o.hostname))throw Error("GROWTH_ORIGIN_INVALID");
  return new URL(path,o.origin).toString();
}
