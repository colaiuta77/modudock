// 동네마트 할인 행사 입력·미리보기·게시 화면을 제공한다.
import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import {
  IconBell,
  IconDeviceFloppy,
  IconHistory,
  IconPlus,
  IconSend,
  IconTrash,
  IconWand,
} from "@tabler/icons-react";

import { api } from "./api";

type SaleItem = {
  id?: string;
  name: string;
  regular_price: number | null;
  sale_price: number;
  unit: string;
  note: string;
};

type PushDelivery = {
  status: "sent" | "failed" | "skipped";
  error: string | null;
  sent_at: string;
} | null;

type SaleBatch = {
  id: string;
  store_id: string;
  sale_date: string;
  title: string;
  raw_text: string;
  status: "draft" | "published" | "expired";
  version: number;
  items: SaleItem[];
  item_count: number;
  published_at: string | null;
  push_delivery: PushDelivery;
};

type SaleSettings = {
  store_id: string;
  store_name: string;
  fcm_enabled: boolean;
  fcm_topic: string;
  fcm_service_account_file: string;
};

const emptySettings: SaleSettings = {
  store_id: "main",
  store_name: "우리동네마트",
  fcm_enabled: false,
  fcm_topic: "mart-main",
  fcm_service_account_file: "",
};

function today() {
  const date = new Date();
  return new Date(date.getTime() - date.getTimezoneOffset() * 60_000)
    .toISOString()
    .slice(0, 10);
}

function emptyDraft(storeId = "main") {
  return {
    id: "",
    store_id: storeId,
    sale_date: today(),
    title: "오늘의 할인",
    raw_text: "",
    items: [] as SaleItem[],
  };
}

export function LocalMartSaleWorkspace({
  token,
  canManage,
  resource = "sales",
  notify,
}: {
  token: string;
  canManage: boolean;
  resource?: string;
  notify: (message: string) => void;
}) {
  const base = `/api/plugins/local-mart-sale/resources/${resource}`;
  const [batches, setBatches] = useState<SaleBatch[]>([]);
  const [settings, setSettings] = useState<SaleSettings>(emptySettings);
  const [draft, setDraft] = useState(() => emptyDraft());
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    const [batchData, settingData] = await Promise.all([
      api<SaleBatch[]>(`${base}/batches`, token),
      api<Partial<SaleSettings>>("/api/plugins/local-mart-sale/settings", token),
    ]);
    const merged = { ...emptySettings, ...settingData };
    setBatches(batchData);
    setSettings(merged);
    setDraft((current) => ({
      ...current,
      store_id: current.id ? current.store_id : merged.store_id,
    }));
  }, [base, token]);

  useEffect(() => {
    void load().catch((error) =>
      notify(error instanceof Error ? error.message : "할인 행사 정보를 불러오지 못했습니다."),
    );
  }, [load, notify]);

  const selected = useMemo(
    () => batches.find((batch) => batch.id === draft.id),
    [batches, draft.id],
  );

  async function parseText() {
    try {
      setBusy(true);
      const parsed = await api<{ items: SaleItem[] }>(`${base}/parse`, token, {
        method: "POST",
        body: JSON.stringify({ raw_text: draft.raw_text }),
      });
      setDraft({ ...draft, items: parsed.items });
      notify(`${parsed.items.length}개 품목을 변환했습니다.`);
    } catch (error) {
      notify(error instanceof Error ? error.message : "텍스트 변환에 실패했습니다.");
    } finally {
      setBusy(false);
    }
  }

  async function save(event: FormEvent) {
    event.preventDefault();
    if (!canManage) return;
    try {
      setBusy(true);
      const saved = await api<SaleBatch>(
        draft.id ? `${base}/batches/${draft.id}` : `${base}/batches`,
        token,
        {
          method: draft.id ? "PUT" : "POST",
          body: JSON.stringify(draft),
        },
      );
      notify("할인 행사 초안을 저장했습니다.");
      await load();
      edit(saved);
    } catch (error) {
      notify(error instanceof Error ? error.message : "초안 저장에 실패했습니다.");
    } finally {
      setBusy(false);
    }
  }

  async function publish() {
    if (!draft.id || !window.confirm("이 할인 행사를 게시하고 푸시 알림 작업을 시작할까요?")) return;
    try {
      setBusy(true);
      await api(`${base}/batches/${draft.id}/publish`, token, { method: "POST" });
      notify("할인 행사를 게시하고 알림 작업을 요청했습니다.");
      await load();
    } catch (error) {
      notify(error instanceof Error ? error.message : "게시하지 못했습니다.");
    } finally {
      setBusy(false);
    }
  }

  async function remove() {
    if (!draft.id || !window.confirm("이 초안을 삭제할까요?")) return;
    try {
      setBusy(true);
      await api(`${base}/batches/${draft.id}`, token, { method: "DELETE" });
      setDraft(emptyDraft(settings.store_id));
      await load();
      notify("초안을 삭제했습니다.");
    } catch (error) {
      notify(error instanceof Error ? error.message : "초안을 삭제하지 못했습니다.");
    } finally {
      setBusy(false);
    }
  }

  async function saveSettings(event: FormEvent) {
    event.preventDefault();
    try {
      await api("/api/plugins/local-mart-sale/settings", token, {
        method: "PUT",
        body: JSON.stringify(settings),
      });
      notify("매장과 알림 설정을 저장했습니다.");
    } catch (error) {
      notify(error instanceof Error ? error.message : "설정을 저장하지 못했습니다.");
    }
  }

  function edit(batch: SaleBatch) {
    setDraft({
      id: batch.id,
      store_id: batch.store_id,
      sale_date: batch.sale_date,
      title: batch.title,
      raw_text: batch.raw_text,
      items: batch.items,
    });
  }

  function updateItem(index: number, field: keyof SaleItem, value: string) {
    setDraft({
      ...draft,
      items: draft.items.map((item, itemIndex) => {
        if (itemIndex !== index) return item;
        if (field === "regular_price") {
          return { ...item, regular_price: value === "" ? null : Number(value) };
        }
        if (field === "sale_price") {
          return { ...item, sale_price: Number(value) };
        }
        return { ...item, [field]: value };
      }),
    });
  }

  return (
    <div className="sale-workspace">
      <form className="sale-editor" onSubmit={save}>
        <div className="section-title">
          <div>
            <p className="eyebrow">SALE EDITOR</p>
            <h3>{draft.id ? "할인 행사 편집" : "새 할인 행사"}</h3>
          </div>
          <button
            type="button"
            className="ghost"
            onClick={() => setDraft(emptyDraft(settings.store_id))}
          >
            <IconPlus size={17} />
            새 초안
          </button>
        </div>
        <div className="form-grid">
          <label>
            행사 날짜
            <input
              type="date"
              value={draft.sale_date}
              onChange={(event) => setDraft({ ...draft, sale_date: event.target.value })}
              disabled={!canManage}
            />
          </label>
          <label>
            제목
            <input
              maxLength={160}
              value={draft.title}
              onChange={(event) => setDraft({ ...draft, title: event.target.value })}
              disabled={!canManage}
            />
          </label>
          <label className="full-field">
            품목 텍스트
            <textarea
              rows={6}
              placeholder={"사과 | 3,000 | 1,990 | 1봉 | 한정 수량\n우유 | 2,800 | 1,980 | 1L"}
              value={draft.raw_text}
              onChange={(event) => setDraft({ ...draft, raw_text: event.target.value })}
              disabled={!canManage}
            />
          </label>
        </div>
        <div className="form-actions split-actions">
          <button type="button" className="ghost" disabled={busy || !canManage} onClick={parseText}>
            <IconWand size={17} />
            텍스트 변환
          </button>
          <div>
            {selected?.status === "draft" && (
              <button type="button" className="danger" disabled={busy} onClick={remove}>
                <IconTrash size={17} />
                삭제
              </button>
            )}
            <button type="submit" className="ghost" disabled={busy || !canManage}>
              <IconDeviceFloppy size={17} />
              초안 저장
            </button>
            <button
              type="button"
              disabled={busy || !canManage || !draft.id || !draft.items.length}
              onClick={publish}
            >
              <IconSend size={17} />
              게시 및 알림
            </button>
          </div>
        </div>
        <div className="table-wrap sale-items">
          <table>
            <thead>
              <tr>
                <th>품목명</th>
                <th>정상가</th>
                <th>할인가</th>
                <th>단위</th>
                <th>설명</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {draft.items.map((item, index) => (
                <tr key={item.id ?? index}>
                  <td><input value={item.name} onChange={(event) => updateItem(index, "name", event.target.value)} /></td>
                  <td><input type="number" min="0" value={item.regular_price ?? ""} onChange={(event) => updateItem(index, "regular_price", event.target.value)} /></td>
                  <td><input type="number" min="0" required value={item.sale_price} onChange={(event) => updateItem(index, "sale_price", event.target.value)} /></td>
                  <td><input value={item.unit} onChange={(event) => updateItem(index, "unit", event.target.value)} /></td>
                  <td><input value={item.note} onChange={(event) => updateItem(index, "note", event.target.value)} /></td>
                  <td>
                    <button
                      type="button"
                      className="icon-button danger"
                      aria-label={`${item.name} 삭제`}
                      onClick={() => setDraft({ ...draft, items: draft.items.filter((_, itemIndex) => itemIndex !== index) })}
                    >
                      <IconTrash size={16} />
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {!draft.items.length && <div className="empty">텍스트를 입력하고 변환하면 품목을 편집할 수 있습니다.</div>}
        </div>
      </form>

      <div className="sale-side">
        <section className="subpanel">
          <div className="section-title">
            <div><p className="eyebrow">HISTORY</p><h3>게시 이력</h3></div>
            <IconHistory size={20} />
          </div>
          <div className="sale-history">
            {batches.map((batch) => (
              <button
                type="button"
                className={batch.id === draft.id ? "selected" : ""}
                key={batch.id}
                onClick={() => edit(batch)}
              >
                <span><strong>{batch.title}</strong><small>{batch.sale_date} · {batch.item_count}개</small></span>
                <span className={`badge ${batch.status}`}>{batch.status}</span>
              </button>
            ))}
            {!batches.length && <div className="empty compact">저장된 행사가 없습니다.</div>}
          </div>
        </section>

        <form className="subpanel sale-settings" onSubmit={saveSettings}>
          <div className="section-title">
            <div><p className="eyebrow">STORE & PUSH</p><h3>매장·알림 설정</h3></div>
            <IconBell size={20} />
          </div>
          <label>매장 ID<input value={settings.store_id} onChange={(event) => setSettings({ ...settings, store_id: event.target.value })} /></label>
          <label>매장 이름<input value={settings.store_name} onChange={(event) => setSettings({ ...settings, store_name: event.target.value })} /></label>
          <label>FCM 주제<input value={settings.fcm_topic} onChange={(event) => setSettings({ ...settings, fcm_topic: event.target.value })} /></label>
          <label>서비스 계정 파일 경로<input placeholder="/run/secrets/firebase.json" value={settings.fcm_service_account_file} onChange={(event) => setSettings({ ...settings, fcm_service_account_file: event.target.value })} /></label>
          <label className="switch-label">
            <input type="checkbox" checked={settings.fcm_enabled} onChange={(event) => setSettings({ ...settings, fcm_enabled: event.target.checked })} />
            FCM 푸시 사용
          </label>
          <button disabled={!canManage}>설정 저장</button>
        </form>
      </div>
    </div>
  );
}
