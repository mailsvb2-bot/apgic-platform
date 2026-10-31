"use client";

import { FormEvent, useEffect, useState } from "react";
import { SpecialistPublicEditor } from "./public-editor";

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

type BookableOffer = {
  id: string;
  product_id: string;
  topic_id: string;
  price_minor: number;
  currency: string;
  state: string;
};

type OwnedProduct = {
  id: string;
  name: string;
  status: "DRAFT" | "PUBLISHED";
};

type OwnedOrganization = {
  id: string;
  name: string;
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
  const [bookableOffers, setBookableOffers] = useState<BookableOffer[]>([]);
  const [ownedProducts, setOwnedProducts] = useState<OwnedProduct[]>([]);
  const [selectedProductID, setSelectedProductID] = useState("");
  const [selectedOfferID, setSelectedOfferID] = useState("");
  const [priceRub, setPriceRub] = useState("");
  const [slotStart, setSlotStart] = useState("");
  const [slotEnd, setSlotEnd] = useState("");

  useEffect(() => {
    void loadProfile();
  }, []);

  useEffect(() => {
    if (profile) {
      void loadBookableWorkspace();
    }
  }, [profile?.id]);

  async function loadProfile() {
    try {
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
        setNotice("Профиль пока не удалось загрузить. Повторите попытку.");
      }
    } catch {
      setNotice("Нет соединения с APGIC. Проверьте сеть и повторите попытку.");
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
    } catch {
      setNotice("Не удалось связаться с APGIC. Данные не подтверждены сервером; повторите попытку.");
      return null;
    } finally {
      setBusy(false);
    }
  }

  async function saveProfile(event: FormEvent) {
    event.preventDefault();
    await callProfile("/v1/specialist/profile", {
      method: "PUT",
      body: JSON.stringify({ display_name: displayName, profession_code: professionCode }),
    }, "Профиль сохранён. В каталоге вы отображаетесь со статусом «Специализация не подтверждена». Чтобы получить подтверждение и возможность записи, добавьте направления и документы.");
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

  async function loadBookableWorkspace() {
    // No invented product or price: discover only authenticated existing
    // Organization-owned PUBLISHED products through the canonical API.
    const [offersReply, organizationsReply] = await Promise.all([
      fetch("/v1/specialist/offers", { cache: "no-store" }),
      fetch("/v1/organizations", { cache: "no-store" }),
    ]);
    if (offersReply.ok) {
      const body = await offersReply.json() as { offers: BookableOffer[] };
      const offers = body.offers ?? [];
      setBookableOffers(offers);
      setSelectedOfferID((current) => current || offers.find((offer) => offer.state === "ACTIVE")?.id || "");
    }
    if (!organizationsReply.ok) return;
    const body = await organizationsReply.json() as { organizations: OwnedOrganization[] };
    const orgs = body.organizations ?? [];
    const responses = await Promise.all(orgs.map(async (org) => {
      const result = await fetch(`/v1/organizations/${encodeURIComponent(org.id)}/products`, { cache: "no-store" });
      if (!result.ok) return [] as OwnedProduct[];
      const values = await result.json() as { products: OwnedProduct[] };
      return (values.products ?? []).filter((product) => product.status === "PUBLISHED");
    }));
    const published = responses.flat();
    setOwnedProducts(published);
    setSelectedProductID((current) => current || published[0]?.id || "");
  }

  async function createPricedOffer(event: FormEvent) {
    event.preventDefault();
    const major = Number(priceRub.replace(",", ".").trim());
    if (!Number.isFinite(major) || major <= 0) {
      setNotice("Введите положительную цену консультации.");
      return;
    }
    setBusy(true);
    try {
      const response = await fetch("/v1/specialist/offers", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({
          topic_id: topicID,
          product_id: selectedProductID,
          price_minor: Math.round(major * 100),
        }),
      });
      const body = await response.json() as BookableOffer & { message_safe?: string };
      if (!response.ok) {
        setNotice(body.message_safe || "Не удалось опубликовать платное предложение.");
        return;
      }
      setBookableOffers((current) => [...current, body]);
      setSelectedOfferID(body.id);
      setNotice("Цена и направление сохранены в PostgreSQL. Добавьте свободное время — только тогда предложение станет доступным в подборе.");
    } finally {
      setBusy(false);
    }
  }

  async function pausePricedOffer() {
    if (!selectedOfferID) return;
    setBusy(true);
    try {
      const response = await fetch(`/v1/specialist/offers/${encodeURIComponent(selectedOfferID)}/pause`, {
        method: "POST",
      });
      const body = await response.json() as BookableOffer & { message_safe?: string };
      if (!response.ok) {
        setNotice(body.message_safe || "Не удалось остановить предложение.");
        return;
      }
      setBookableOffers((current) => current.map((offer) => (
        offer.id === body.id ? body : offer
      )));
      setSelectedOfferID("");
      setNotice("Старая цена снята с публикации без удаления записей. Теперь можно создать новое предложение с другой ценой.");
    } finally {
      setBusy(false);
    }
  }

  async function createBookableSlot(event: FormEvent) {
    event.preventDefault();
    const start = new Date(slotStart);
    const end = new Date(slotEnd);
    if (!Number.isFinite(start.getTime()) || !Number.isFinite(end.getTime()) || end <= start) {
      setNotice("Укажите корректное начало и окончание консультации.");
      return;
    }
    setBusy(true);
    try {
      const response = await fetch(`/v1/specialist/offers/${encodeURIComponent(selectedOfferID)}/slots`, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ starts_at: start.toISOString(), ends_at: end.toISOString() }),
      });
      const body = await response.json() as { message_safe?: string };
      if (!response.ok) {
        setNotice(body.message_safe || "Не удалось сохранить время консультации.");
        return;
      }
      setSlotStart("");
      setSlotEnd("");
      setNotice("Свободное время сохранено в PostgreSQL. Теперь клиент может увидеть цену и удержать этот слот для записи. Реальная оплата остаётся закрыта до подключения PSP.");
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

      {profile ? <section className="panel" aria-label="Кабинет специалиста">
        <p className="section-kicker">Личный кабинет специалиста</p>
        <h3>{profile.display_name}</h3>
        <p>{profile.profession_code === "PSYCHOLOGIST" ? "Психолог" : profile.profession_code === "CAREER_COACH" ? "Карьерный консультант" : "Специалист"}</p>
        <p role="status"><strong>{profile.published_topics.length > 0 ? "Профессионализм подтверждён по опубликованным направлениям" : "Специализация не подтверждена"}</strong></p>
        <p>Ваша анкета сохраняется в APGIC. Без подтверждения квалификации она отображается среди зарегистрированных специалистов; платная запись станет доступна после проверки.</p>
        <nav aria-label="Разделы кабинета специалиста">
          <a href="#specialist-profile">Моя анкета</a>{" · "}
          <a href="#specialist-topics">Направления</a>{" · "}
          <a href="#specialist-documents">Документы</a>{" · "}
          <a href="#specialist-publication">Подтверждение</a>{" · "}
          <a href="#specialist-offers">Услуги и расписание</a>{" · "}
          <a href="#specialist-consultations">Консультации</a>{" · "}
          <a href="/specialists">Каталог специалистов</a>
        </nav>
        <p className="form-help">Статус проверки профиля: {profile.review_state}. Подтверждённые направления: {profile.published_topics.length ? profile.published_topics.map(t => topicNames[t] || t).join(", ") : "пока нет"}.</p>
      </section> : null}
      <div className="specialist-workspace">
        <p className="onboarding-notice" role="status">{notice}</p>

        <form className="panel specialist-form" id="specialist-profile" onSubmit={saveProfile}>
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
            <SpecialistPublicEditor />
            <section className="panel specialist-form" id="specialist-topics" aria-labelledby="capability-title">
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

            <form className="panel specialist-form" id="specialist-documents" onSubmit={submitEvidence}>
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

            <section className="panel specialist-form" id="specialist-publication" aria-labelledby="publish-title">
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
            <section className="panel specialist-form" id="specialist-offers" aria-labelledby="offer-title">
              <h3 id="offer-title">5. Реальная цена и доступное время</h3>
              <p className="form-help">
                Для платной консультации нужен уже опубликованный продукт вашей организации,
                действующее подтверждение компетенции и одобренный профиль. Тестовых цен и слотов нет.
              </p>
              {ownedProducts.length === 0 ? (
                <p>Опубликованных продуктов пока нет. <a href="/organization">Создать и опубликовать продукт в кабинете организации</a>.</p>
              ) : (
                <form onSubmit={createPricedOffer}>
                  <label htmlFor="specialist-owned-product">Опубликованный продукт</label>
                  <select id="specialist-owned-product" value={selectedProductID}
                    onChange={(event) => setSelectedProductID(event.target.value)}>
                    {ownedProducts.map((product) => (
                      <option key={product.id} value={product.id}>{product.name}</option>
                    ))}
                  </select>
                  <label htmlFor="specialist-priced-amount">Цена консультации, ₽</label>
                  <input id="specialist-priced-amount" value={priceRub} inputMode="decimal"
                    onChange={(event) => setPriceRub(event.target.value)} placeholder="Например, 2500"
                    required />
                  <button type="submit" disabled={busy || !selectedProductID ||
                    !profile.published_topics.includes(topicID)}>Сохранить платное предложение</button>
                </form>
              )}
              <label htmlFor="specialist-bookable-offer">Предложение для свободного времени</label>
              <select id="specialist-bookable-offer" value={selectedOfferID}
                onChange={(event) => setSelectedOfferID(event.target.value)}>
                <option value="">Выберите опубликованное предложение</option>
                {bookableOffers.filter((offer) => offer.state === "ACTIVE").map((offer) => (
                  <option key={offer.id} value={offer.id}>
                    {topicNames[offer.topic_id] || offer.topic_id} ·
                    {" "}{new Intl.NumberFormat("ru-RU").format(offer.price_minor / 100)} ₽
                  </option>
                ))}
              </select>
              <button type="button" className="secondary-panel-action" onClick={pausePricedOffer}
                disabled={busy || !selectedOfferID}>
                Остановить предложение и изменить цену
              </button>
              <form onSubmit={createBookableSlot}>
                <label htmlFor="specialist-slot-start">Начало (местное время)</label>
                <input id="specialist-slot-start" type="datetime-local" value={slotStart}
                  onChange={(event) => setSlotStart(event.target.value)} required />
                <label htmlFor="specialist-slot-end">Окончание (местное время)</label>
                <input id="specialist-slot-end" type="datetime-local" value={slotEnd}
                  onChange={(event) => setSlotEnd(event.target.value)} required />
                <button type="submit" disabled={busy || !selectedOfferID}>Добавить доступное время</button>
              </form>
            </section>
          </>
        )}
      </div>
    </section>
  );
}
