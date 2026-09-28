"use client";

import { FormEvent, useEffect, useState } from "react";

type Capability = {
  topic_id: string;
  evidence_state: string;
  verification_state: string;
  evidence_refs: string[];
};

type Evidence = {
  id: string;
  topic_id: string;
  kind: string;
  reference: string;
  state: string;
};

type Profile = {
  id: string;
  identity_id: string;
  display_name: string;
  profession_code: string;
  profile_complete: boolean;
  review_state: string;
  capabilities: Capability[];
  evidence: Evidence[];
  published_topics: string[];
};

type PublishResult = {
  allowed: boolean;
  reason_codes: string[];
  published_topics: string[];
  policy_version: string;
};

const topicNames: Record<string, string> = {
  anxiety: "Тревога и стресс",
  sleep: "Сон",
  relationships: "Отношения",
  career: "Карьера и работа",
};

function safeMessage(code?: string) {
  switch (code) {
    case "SPECIALIST_PROFILE_NOT_FOUND":
      return "Профиль ещё не создан.";
    case "SPECIALIST_CAPABILITY_NOT_FOUND":
      return "Сначала добавьте направление работы.";
    case "SPECIALIST_PROFILE_INVALID":
      return "Заполните имя и профессию.";
    case "SPECIALIST_EVIDENCE_INVALID":
      return "Укажите тип и ссылку или номер подтверждения.";
    default:
      return "Не удалось выполнить действие. Попробуйте ещё раз.";
  }
}

export function SpecialistOnboarding() {
  const [profile, setProfile] = useState<Profile | null>(null);
  const [displayName, setDisplayName] = useState("");
  const [professionCode, setProfessionCode] = useState("PSYCHOLOGIST");
  const [topicID, setTopicID] = useState("anxiety");
  const [evidenceKind, setEvidenceKind] = useState("DIPLOMA");
  const [evidenceReference, setEvidenceReference] = useState("");
  const [notice, setNotice] = useState("Создайте профиль, чтобы начать подключение.");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    void loadProfile();
  }, []);

  async function loadProfile() {
    const response = await fetch("/v1/specialist/profile", { cache: "no-store" });
    if (response.ok) {
      const loaded = (await response.json()) as Profile;
      setProfile(loaded);
      setDisplayName(loaded.display_name);
      setProfessionCode(loaded.profession_code);
      setNotice("Профиль загружен.");
      return;
    }
    if (response.status !== 401 && response.status !== 404) {
      setNotice("Профиль пока не удалось загрузить.");
    }
  }

  async function callProfile(path: string, init: RequestInit, success: string) {
    setBusy(true);
    try {
      const response = await fetch(path, {
        ...init,
        headers: { "content-type": "application/json", ...(init.headers || {}) },
      });
      const body = await response.json();
      if (!response.ok) {
        setNotice(body?.message_safe || safeMessage(body?.code));
        return null;
      }
      const updated = body as Profile;
      setProfile(updated);
      setDisplayName(updated.display_name);
      setProfessionCode(updated.profession_code);
      setNotice(success);
      return updated;
    } finally {
      setBusy(false);
    }
  }

  async function saveProfile(event: FormEvent) {
    event.preventDefault();
    await callProfile("/v1/specialist/profile", {
      method: "PUT",
      body: JSON.stringify({ display_name: displayName, profession_code: professionCode }),
    }, "Профиль сохранён. Теперь добавьте направления работы.");
  }

  async function declareCapability() {
    await callProfile("/v1/specialist/capabilities", {
      method: "POST",
      body: JSON.stringify({ topic_id: topicID }),
    }, "Направление добавлено как SELF_DECLARED — это ещё не верификация.");
  }

  async function submitEvidence(event: FormEvent) {
    event.preventDefault();
    const updated = await callProfile("/v1/specialist/evidence", {
      method: "POST",
      body: JSON.stringify({
        topic_id: topicID,
        kind: evidenceKind,
        reference: evidenceReference,
      }),
    }, "Подтверждение принято и отправлено на review. Публикация пока заблокирована.");
    if (updated) {
      setEvidenceReference("");
    }
  }

  async function publish() {
    setBusy(true);
    try {
      const response = await fetch("/v1/specialist/publish", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ topic_id: topicID }),
      });
      const body = (await response.json()) as PublishResult & { message_safe?: string; code?: string };
      if (!response.ok && response.status !== 409) {
        setNotice(body.message_safe || safeMessage(body.code));
        return;
      }
      if (!body.allowed) {
        setNotice(`Публикация пока заблокирована: ${(body.reason_codes || []).join(", ")}.`);
        return;
      }
      setNotice("Направление опубликовано и может участвовать в подборе.");
      await loadProfile();
    } finally {
      setBusy(false);
    }
  }

  async function unpublish() {
    setBusy(true);
    try {
      const response = await fetch("/v1/specialist/unpublish", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ topic_id: topicID }),
      });
      if (!response.ok) {
        const body = await response.json();
        setNotice(body?.message_safe || safeMessage(body?.code));
        return;
      }
      setNotice("Направление снято с публикации.");
      await loadProfile();
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="specialist-onboarding" id="onboarding" aria-labelledby="onboarding-title">
      <div className="start-heading">
        <p className="section-kicker">Подключение</p>
        <h2 id="onboarding-title">Создайте профессиональный профиль</h2>
        <p>
          Все изменения сохраняются в APGIC. Заявленные направления не получают статус APGIC VERIFIED
          до доверенного review.
        </p>
      </div>

      <div className="specialist-workspace">
        <p className="onboarding-notice" role="status">{notice}</p>

        <form className="panel specialist-form" onSubmit={saveProfile}>
          <h3>1. Профиль</h3>
          <label htmlFor="specialist-name">Имя для публичного профиля</label>
          <input
            id="specialist-name"
            value={displayName}
            onChange={(event) => setDisplayName(event.target.value)}
            placeholder="Например, Анна Петрова"
            required
          />
          <label htmlFor="specialist-profession">Профессия</label>
          <select
            id="specialist-profession"
            value={professionCode}
            onChange={(event) => setProfessionCode(event.target.value)}
          >
            <option value="PSYCHOLOGIST">Психолог</option>
            <option value="CAREER_COACH">Карьерный консультант</option>
          </select>
          <button type="submit" disabled={busy}>Сохранить профиль</button>
        </form>

        {profile && (
          <>
            <section className="panel specialist-form" aria-labelledby="capability-title">
              <h3 id="capability-title">2. Направления работы</h3>
              <label htmlFor="specialist-topic">Направление</label>
              <select id="specialist-topic" value={topicID} onChange={(event) => setTopicID(event.target.value)}>
                {Object.entries(topicNames).map(([value, label]) => (
                  <option key={value} value={value}>{label}</option>
                ))}
              </select>
              <button type="button" onClick={declareCapability} disabled={busy}>Добавить направление</button>
              <ul className="specialist-state-list">
                {profile.capabilities.map((capability) => (
                  <li key={capability.topic_id}>
                    <strong>{topicNames[capability.topic_id] || capability.topic_id}</strong>
                    <span>{capability.evidence_state} · {capability.verification_state}</span>
                  </li>
                ))}
              </ul>
            </section>

            <form className="panel specialist-form" onSubmit={submitEvidence}>
              <h3>3. Подтверждение компетенции</h3>
              <p className="form-help">
                Сейчас APGIC принимает ссылку или номер подтверждающего материала. Сами файлы через этот экран не загружаются.
              </p>
              <label htmlFor="evidence-kind">Тип подтверждения</label>
              <select id="evidence-kind" value={evidenceKind} onChange={(event) => setEvidenceKind(event.target.value)}>
                <option value="DIPLOMA">Диплом</option>
                <option value="CERTIFICATE">Сертификат</option>
                <option value="LICENSE">Лицензия / реестровая запись</option>
                <option value="OTHER">Другое</option>
              </select>
              <label htmlFor="evidence-reference">Ссылка или номер подтверждения</label>
              <input
                id="evidence-reference"
                value={evidenceReference}
                onChange={(event) => setEvidenceReference(event.target.value)}
                placeholder="https://… или номер документа"
                required
              />
              <button type="submit" disabled={busy}>Передать на проверку</button>
              <ul className="specialist-state-list">
                {profile.evidence.map((item) => (
                  <li key={item.id}>
                    <strong>{item.kind}</strong>
                    <span>{item.state} · {topicNames[item.topic_id] || item.topic_id}</span>
                  </li>
                ))}
              </ul>
            </form>

            <section className="panel specialist-form" aria-labelledby="publish-title">
              <h3 id="publish-title">4. Публикация</h3>
              <p>
                Review профиля: <strong>{profile.review_state}</strong>. Публикация разрешается только после
                проверки профиля и требуемой компетенции.
              </p>
              <div className="specialist-actions">
                <button type="button" onClick={publish} disabled={busy}>Проверить и опубликовать</button>
                <button className="secondary-panel-action" type="button" onClick={unpublish} disabled={busy}>
                  Снять с публикации
                </button>
              </div>
              <p className="form-help">
                Опубликовано: {profile.published_topics.length
                  ? profile.published_topics.map((topic) => topicNames[topic] || topic).join(", ")
                  : "пока ничего"}.
              </p>
            </section>
          </>
        )}
      </div>
    </section>
  );
}
