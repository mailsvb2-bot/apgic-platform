import {parseClientPersonalView, type ClientPersonalPreferences, type ClientPersonalView} from "../../../packages/contracts/src/client-personal";

export class PersonalAPIError extends Error {
  constructor(public readonly reason: "SESSION"|"UNAVAILABLE"|"INVALID", message: string) {super(message);}
}
async function check(response: Response): Promise<void> {
  if (response.status === 401 || response.status === 403) {
    throw new PersonalAPIError("SESSION", "Нет подтверждённой сессии. Начните с оформления запроса на главной странице.");
  }
  if (!response.ok) throw new PersonalAPIError("UNAVAILABLE","Сейчас не удалось получить данные. Повторите попытку.");
}
export async function loadClientPersonal(cursor = "", signal?: AbortSignal): Promise<ClientPersonalView> {
  const query = cursor ? `?history_cursor=${encodeURIComponent(cursor)}` : "";
  const response = await fetch("/v1/client/personal" + query, {
    credentials: "include", cache: "no-store", headers: {Accept:"application/json"}, signal,
  });
  await check(response);
  try { return parseClientPersonalView(await response.json()); }
  catch { throw new PersonalAPIError("INVALID","Сервер вернул некорректную историю."); }
}
export async function saveClientPersonalPreferences(p: ClientPersonalPreferences): Promise<ClientPersonalPreferences> {
  const response=await fetch("/v1/client/personal/preferences", {
    method:"PUT",credentials:"include",cache:"no-store",
    headers:{"content-type":"application/json",Accept:"application/json"},
    body:JSON.stringify({display_name:p.display_name,locale:p.locale,time_zone:p.time_zone}),
  });
  await check(response);
  const value: unknown = await response.json();
  if (!value || typeof value!=="object" ||
    (value as ClientPersonalPreferences).display_name !== p.display_name ||
    (value as ClientPersonalPreferences).locale !== p.locale ||
    (value as ClientPersonalPreferences).time_zone !== p.time_zone) {
    throw new PersonalAPIError("INVALID","Настройки не подтверждены сервером.");
  }
  return value as ClientPersonalPreferences;
}

export async function recordPersonalCheckIn(input: import("../../../packages/contracts/src/client-personal").ClientPersonalCheckInInput): Promise<void> {
  const response=await fetch("/v1/client/personal/check-ins",{
    method:"POST",credentials:"include",cache:"no-store",
    headers:{"content-type":"application/json",Accept:"application/json"},
    body:JSON.stringify(input),
  });
  await check(response);
  const reply=await response.json() as {booking_id?:string,progress_code?:string};
  if(reply.booking_id!==input.booking_id || reply.progress_code!==input.progress_code)
    throw new PersonalAPIError("INVALID","Сервер не подтвердил сохранение оценки.");
}
