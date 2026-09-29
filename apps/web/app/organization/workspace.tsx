"use client";

import { FormEvent, useState } from "react";

type Direction = {
  id: string;
  organization_id: string;
  name: string;
  direction_type: string;
  status: string;
};

type Organization = {
  id: string;
  name: string;
  status: string;
  directions: Direction[];
};

async function requestJSON<T>(path: string, init: RequestInit): Promise<T> {
  const response = await fetch(path, {
    ...init,
    headers: { "content-type": "application/json", ...(init.headers || {}) },
  });
  const payload = await response.json();
  if (!response.ok) {
    throw new Error(payload?.message_safe || "Не удалось выполнить действие.");
  }
  return payload as T;
}

export function OrganizationWorkspace() {
  const [organization, setOrganization] = useState<Organization | null>(null);
  const [organizationName, setOrganizationName] = useState("");
  const [directionName, setDirectionName] = useState("");
  const [directionType, setDirectionType] = useState("SERVICE");
  const [notice, setNotice] = useState("Создайте организацию, чтобы открыть рабочее пространство.");
  const [busy, setBusy] = useState(false);

  async function createOrganization(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    try {
      const created = await requestJSON<Organization>("/v1/organizations", {
        method: "POST",
        body: JSON.stringify({ name: organizationName }),
      });
      setOrganization(created);
      setOrganizationName(created.name);
      setNotice("Организация создана. Вы добавлены как активный владелец.");
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "Не удалось создать организацию.");
    } finally {
      setBusy(false);
    }
  }

  async function createDirection(event: FormEvent) {
    event.preventDefault();
    if (!organization) return;
    setBusy(true);
    try {
      const updated = await requestJSON<Organization>(`/v1/organizations/${organization.id}/directions`, {
        method: "POST",
        body: JSON.stringify({ name: directionName, direction_type: directionType }),
      });
      setOrganization(updated);
      setDirectionName("");
      setNotice("Направление создано.");
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "Не удалось создать направление.");
    } finally {
      setBusy(false);
    }
  }

  async function archiveDirection(direction: Direction) {
    if (!organization) return;
    setBusy(true);
    try {
      const updated = await requestJSON<Organization>(
        `/v1/organizations/${organization.id}/directions/${direction.id}/archive`,
        { method: "POST" },
      );
      setOrganization(updated);
      setNotice(`Направление «${direction.name}» архивировано. История сохранена.`);
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "Не удалось архивировать направление.");
    } finally {
      setBusy(false);
    }
  }

  const active = organization?.directions.filter((item) => item.status === "ACTIVE") ?? [];
  const archived = organization?.directions.filter((item) => item.status === "ARCHIVED") ?? [];

  return (
    <section className="organization-workspace" aria-labelledby="organization-workspace-title">
      <div className="start-heading">
        <p className="section-kicker">Рабочее пространство</p>
        <h2 id="organization-workspace-title">Организация и направления</h2>
        <p>Все действия выполняются через реальный APGIC API и текущую подписанную сессию пользователя.</p>
      </div>

      <div className="organization-panels">
        <p className="onboarding-notice" role="status">{notice}</p>

        {!organization ? (
          <form className="panel specialist-form" onSubmit={createOrganization}>
            <h3>1. Создать организацию</h3>
            <label htmlFor="organization-name">Название организации</label>
            <input
              id="organization-name"
              value={organizationName}
              onChange={(event) => setOrganizationName(event.target.value)}
              placeholder="Например, Центр развития"
              required
            />
            <button type="submit" disabled={busy}>Создать организацию</button>
          </form>
        ) : (
          <>
            <section className="panel organization-summary" aria-labelledby="organization-summary-title">
              <h3 id="organization-summary-title">{organization.name}</h3>
              <div className="organization-meta">
                <span>Статус: <strong>{organization.status}</strong></span>
                <span>ID: <code>{organization.id}</code></span>
              </div>
            </section>

            <form className="panel specialist-form" onSubmit={createDirection}>
              <h3>2. Добавить направление</h3>
              <label htmlFor="direction-name">Название направления</label>
              <input
                id="direction-name"
                value={directionName}
                onChange={(event) => setDirectionName(event.target.value)}
                placeholder="Например, Психологическая помощь"
                required
              />
              <label htmlFor="direction-type">Тип направления</label>
              <select
                id="direction-type"
                value={directionType}
                onChange={(event) => setDirectionType(event.target.value)}
              >
                <option value="SERVICE">Услуги</option>
                <option value="EDUCATION">Обучение</option>
                <option value="CONSULTING">Консалтинг</option>
                <option value="GENERAL">Общее</option>
              </select>
              <button type="submit" disabled={busy}>Добавить направление</button>
            </form>

            <section className="panel" aria-labelledby="active-directions-title">
              <h3 id="active-directions-title">Активные направления</h3>
              {active.length ? (
                <ul className="organization-directions">
                  {active.map((direction) => (
                    <li key={direction.id}>
                      <div>
                        <strong>{direction.name}</strong>
                        <span>{direction.direction_type} · {direction.status}</span>
                      </div>
                      <button type="button" disabled={busy} onClick={() => archiveDirection(direction)}>
                        Архивировать
                      </button>
                    </li>
                  ))}
                </ul>
              ) : <p className="form-help">Активных направлений пока нет.</p>}
            </section>

            {archived.length ? (
              <section className="panel" aria-labelledby="archived-directions-title">
                <h3 id="archived-directions-title">Архив</h3>
                <ul className="organization-directions archived-directions">
                  {archived.map((direction) => (
                    <li key={direction.id}>
                      <div>
                        <strong>{direction.name}</strong>
                        <span>{direction.direction_type} · {direction.status}</span>
                      </div>
                    </li>
                  ))}
                </ul>
              </section>
            ) : null}
          </>
        )}
      </div>
    </section>
  );
}
