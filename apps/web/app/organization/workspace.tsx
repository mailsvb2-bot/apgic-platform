"use client";

import { FormEvent, useEffect, useState } from "react";

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

type Product = {
  id: string;
  name: string;
  status: "DRAFT" | "PUBLISHED";
  owner_type: "ORGANIZATION";
  owner_id: string;
  commercial_owner_ref: string;
  author_refs: string[];
  revenue_beneficiary_ref: string;
  organization_direction_id: string;
  published_at?: string;
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
  const [organizations, setOrganizations] = useState<Organization[]>([]);
  const [organizationName, setOrganizationName] = useState("");
  const [directionName, setDirectionName] = useState("");
  const [directionType, setDirectionType] = useState("SERVICE");
  const [notice, setNotice] = useState("Создайте организацию, чтобы открыть рабочее пространство.");
  const [busy, setBusy] = useState(false);
  const [products, setProducts] = useState<Product[]>([]);
  const [productName, setProductName] = useState("");
  const [productDirectionID, setProductDirectionID] = useState("");
  const [commercialOwnerRef, setCommercialOwnerRef] = useState("");
  const [authorRefs, setAuthorRefs] = useState("");
  const [revenueBeneficiaryRef, setRevenueBeneficiaryRef] = useState("");

  useEffect(() => {
    void loadOrganizations();
  }, []);

  async function loadOrganizations() {
    try {
      const response = await fetch("/v1/organizations", { cache: "no-store" });
      if (response.status === 401) return;
      const payload = await response.json();
      if (!response.ok) {
        setNotice(payload?.message_safe || "Организации пока не удалось загрузить.");
        return;
      }
      const loaded = (payload.organizations || []) as Organization[];
      setOrganizations(loaded);
      if (loaded.length) {
        setOrganization(loaded[0]);
        await loadProducts(loaded[0].id);
        setNotice("Организации загружены.");
      } else {
        setProducts([]);
      }
    } catch {
      setNotice("Организации пока не удалось загрузить.");
    }
  }

  async function loadProducts(organizationID: string) {
    try {
      const response = await fetch(`/v1/organizations/${organizationID}/products`, { cache: "no-store" });
      if (response.status === 401) return;
      const payload = await response.json();
      if (!response.ok) {
        setNotice(payload?.message_safe || "Продукты пока не удалось загрузить.");
        return;
      }
      setProducts((payload.products || []) as Product[]);
    } catch {
      setNotice("Продукты пока не удалось загрузить.");
    }
  }

  async function createOrganization(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    try {
      const created = await requestJSON<Organization>("/v1/organizations", {
        method: "POST",
        body: JSON.stringify({ name: organizationName }),
      });
      setOrganization(created);
      setOrganizations((current) => [...current.filter((item) => item.id !== created.id), created]);
      setOrganizationName(created.name);
      setProducts([]);
      setCommercialOwnerRef(`organization/${created.id}`);
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
      setOrganizations((current) => current.map((item) => item.id === updated.id ? updated : item));
      setDirectionName("");
      const createdDirection = updated.directions.find((item) => item.status === "ACTIVE" && item.name === directionName);
      if (createdDirection && !productDirectionID) setProductDirectionID(createdDirection.id);
      setNotice("Направление создано.");
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "Не удалось создать направление.");
    } finally {
      setBusy(false);
    }
  }

  async function createProduct(event: FormEvent) {
    event.preventDefault();
    if (!organization) return;
    setBusy(true);
    try {
      const created = await requestJSON<Product>(`/v1/organizations/${organization.id}/products`, {
        method: "POST",
        body: JSON.stringify({
          name: productName,
          direction_id: productDirectionID,
          commercial_owner_ref: commercialOwnerRef,
          author_refs: authorRefs.split(",").map((item) => item.trim()).filter(Boolean),
          revenue_beneficiary_ref: revenueBeneficiaryRef,
        }),
      });
      setProducts((current) => [...current.filter((item) => item.id !== created.id), created]);
      setProductName("");
      setNotice(`Продукт «${created.name}» создан как черновик.`);
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "Не удалось создать продукт.");
    } finally {
      setBusy(false);
    }
  }

  async function publishProduct(product: Product) {
    if (!organization) return;
    setBusy(true);
    try {
      const published = await requestJSON<Product>(
        `/v1/organizations/${organization.id}/products/${product.id}/publish`,
        { method: "POST" },
      );
      setProducts((current) => current.map((item) => item.id === published.id ? published : item));
      setNotice(`Продукт «${published.name}» опубликован.`);
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "Не удалось опубликовать продукт.");
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
      setOrganizations((current) => current.map((item) => item.id === updated.id ? updated : item));
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

        {organizations.length > 1 ? (
          <section className="panel specialist-form" aria-labelledby="organization-switch-title">
            <h3 id="organization-switch-title">Ваши организации</h3>
            <label htmlFor="organization-switch">Текущая организация</label>
            <select
              id="organization-switch"
              value={organization?.id || ""}
              onChange={(event) => {
                const selected = organizations.find((item) => item.id === event.target.value) || null;
                setOrganization(selected);
                setProducts([]);
                if (selected) {
                  setCommercialOwnerRef(`organization/${selected.id}`);
                  setProductDirectionID("");
                  void loadProducts(selected.id);
                  setNotice(`Открыта организация «${selected.name}».`);
                }
              }}
            >
              {organizations.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}
            </select>
          </section>
        ) : null}

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

            <form className="panel specialist-form" onSubmit={createProduct}>
              <h3>3. Создать продукт</h3>
              <p className="form-help">
                Публикация разрешена только при явных ролях владельца, коммерческого владельца, авторов и получателя выручки.
              </p>
              <label htmlFor="product-name">Название продукта</label>
              <input
                id="product-name"
                value={productName}
                onChange={(event) => setProductName(event.target.value)}
                placeholder="Например, Первичная консультация"
                required
              />
              <label htmlFor="product-direction">Направление продукта</label>
              <select
                id="product-direction"
                value={productDirectionID}
                onChange={(event) => setProductDirectionID(event.target.value)}
                required
              >
                <option value="">Выберите активное направление</option>
                {active.map((direction) => (
                  <option key={direction.id} value={direction.id}>{direction.name}</option>
                ))}
              </select>
              <label htmlFor="commercial-owner-ref">Коммерческий владелец</label>
              <input
                id="commercial-owner-ref"
                value={commercialOwnerRef}
                onChange={(event) => setCommercialOwnerRef(event.target.value)}
                placeholder={organization ? `organization/${organization.id}` : "organization/..."}
                required
              />
              <label htmlFor="author-refs">Авторы</label>
              <input
                id="author-refs"
                value={authorRefs}
                onChange={(event) => setAuthorRefs(event.target.value)}
                placeholder="identity/...; несколько — через запятую"
                required
              />
              <label htmlFor="revenue-beneficiary-ref">Получатель выручки</label>
              <input
                id="revenue-beneficiary-ref"
                value={revenueBeneficiaryRef}
                onChange={(event) => setRevenueBeneficiaryRef(event.target.value)}
                placeholder="identity/... или organization/..."
                required
              />
              <button type="submit" disabled={busy || active.length === 0}>Создать черновик продукта</button>
            </form>

            <section className="panel" aria-labelledby="organization-products-title">
              <h3 id="organization-products-title">Продукты организации</h3>
              {products.length ? (
                <ul className="organization-products">
                  {products.map((product) => (
                    <li key={product.id}>
                      <div>
                        <strong>{product.name}</strong>
                        <span>{product.status} · направление {product.organization_direction_id}</span>
                        <span>Коммерческий владелец: {product.commercial_owner_ref}</span>
                        <span>Авторы: {product.author_refs.join(", ")}</span>
                        <span>Получатель выручки: {product.revenue_beneficiary_ref}</span>
                      </div>
                      {product.status === "DRAFT" ? (
                        <button type="button" disabled={busy} onClick={() => publishProduct(product)}>
                          Опубликовать
                        </button>
                      ) : (
                        <span className="product-published">Опубликован</span>
                      )}
                    </li>
                  ))}
                </ul>
              ) : <p className="form-help">Продуктов пока нет.</p>}
            </section>

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
