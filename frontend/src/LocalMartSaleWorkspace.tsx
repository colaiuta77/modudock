// 동네마트 세일 원문 작성·게시 이력·알림 설정 화면을 제공한다.
import { ChangeEvent, FormEvent, useCallback, useEffect, useState } from "react";
import {
  IconBell,
  IconDeviceFloppy,
  IconFileUpload,
  IconHistory,
  IconPencil,
  IconPlus,
  IconSend,
  IconTrash,
} from "@tabler/icons-react";

import { api } from "./api";

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

type SaleTab = "editor" | "history" | "settings";

const MAX_TEXT_LENGTH = 50_000;
const emptySettings: SaleSettings = {
  store_id: "main",
  store_name: "우리동네마트",
  fcm_enabled: false,
  fcm_topic: "mart-main",
  fcm_service_account_file: "",
};

const statusLabels: Record<SaleBatch["status"], string> = {
  draft: "초안",
  published: "게시됨",
  expired: "종료됨",
};

const deliveryLabels: Record<NonNullable<PushDelivery>["status"], string> = {
  sent: "발송 완료",
  failed: "발송 실패",
  skipped: "발송 안 함",
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
  };
}

function formatDateTime(value: string | null) {
  if (!value) return "-";
  return new Intl.DateTimeFormat("ko-KR", {
    dateStyle: "short",
    timeStyle: "short",
  }).format(new Date(value));
}

function normalizeText(value: string) {
  return value.replace(/\r\n/g, "\n").replace(/\r/g, "\n");
}

async function readTextFile(file: File) {
  const buffer = await file.arrayBuffer();
  let text: string;
  try {
    text = new TextDecoder("utf-8", { fatal: true }).decode(buffer);
  } catch {
    text = new TextDecoder("euc-kr").decode(buffer);
  }
  const normalized = normalizeText(text).replace(/^\uFEFF/, "");
  if (normalized.length > MAX_TEXT_LENGTH) {
    throw new Error("TXT 파일 내용은 50,000자 이하여야 합니다.");
  }
  return normalized;
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
  const [activeTab, setActiveTab] = useState<SaleTab>("editor");
  const [batches, setBatches] = useState<SaleBatch[]>([]);
  const [settings, setSettings] = useState<SaleSettings>(emptySettings);
  const [draft, setDraft] = useState(() => emptyDraft());
  const [fileName, setFileName] = useState("");
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

  async function handleFile(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    event.target.value = "";
    if (!file) return;
    if (!file.name.toLowerCase().endsWith(".txt")) {
      notify("TXT 파일만 선택할 수 있습니다.");
      return;
    }
    try {
      const rawText = await readTextFile(file);
      setDraft((current) => ({ ...current, raw_text: rawText }));
      setFileName(file.name);
      notify(`${file.name} 내용을 불러왔습니다.`);
    } catch (error) {
      notify(error instanceof Error ? error.message : "TXT 파일을 읽지 못했습니다.");
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
          body: JSON.stringify({ ...draft, items: [] }),
        },
      );
      setDraft({
        id: saved.id,
        store_id: saved.store_id,
        sale_date: saved.sale_date,
        title: saved.title,
        raw_text: saved.raw_text,
      });
      await load();
      notify("세일 원문 초안을 저장했습니다.");
    } catch (error) {
      notify(error instanceof Error ? error.message : "초안을 저장하지 못했습니다.");
    } finally {
      setBusy(false);
    }
  }

  async function publish() {
    if (!draft.id || !window.confirm("이 세일 원문을 게시하고 푸시 알림 작업을 시작할까요?")) return;
    try {
      setBusy(true);
      await api(`${base}/batches/${draft.id}/publish`, token, { method: "POST" });
      await load();
      setActiveTab("history");
      notify("세일 원문을 게시하고 알림 작업을 요청했습니다.");
    } catch (error) {
      notify(error instanceof Error ? error.message : "게시하지 못했습니다.");
    } finally {
      setBusy(false);
    }
  }

  async function remove(batch: SaleBatch) {
    if (!window.confirm(`'${batch.title}' 초안을 삭제할까요?`)) return;
    try {
      setBusy(true);
      await api(`${base}/batches/${batch.id}`, token, { method: "DELETE" });
      if (draft.id === batch.id) {
        setDraft(emptyDraft(settings.store_id));
        setFileName("");
      }
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
    if (!canManage) return;
    try {
      setBusy(true);
      await api("/api/plugins/local-mart-sale/settings", token, {
        method: "PUT",
        body: JSON.stringify(settings),
      });
      notify("매장과 알림 설정을 저장했습니다.");
    } catch (error) {
      notify(error instanceof Error ? error.message : "설정을 저장하지 못했습니다.");
    } finally {
      setBusy(false);
    }
  }

  function edit(batch: SaleBatch) {
    setDraft({
      id: batch.id,
      store_id: batch.store_id,
      sale_date: batch.sale_date,
      title: batch.title,
      raw_text: batch.raw_text,
    });
    setFileName("");
    setActiveTab("editor");
  }

  function startNewDraft() {
    setDraft(emptyDraft(settings.store_id));
    setFileName("");
    setActiveTab("editor");
  }

  return (
    <div className="sale-workspace">
      <div className="sale-tabs" role="tablist" aria-label="오늘의 세일 관리">
        <button
          type="button"
          role="tab"
          aria-selected={activeTab === "editor"}
          className={activeTab === "editor" ? "active" : ""}
          onClick={() => setActiveTab("editor")}
        >
          <IconPencil size={18} />
          세일 작성
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={activeTab === "history"}
          className={activeTab === "history" ? "active" : ""}
          onClick={() => setActiveTab("history")}
        >
          <IconHistory size={18} />
          게시 이력
          <span className="sale-tab-count">{batches.length}</span>
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={activeTab === "settings"}
          className={activeTab === "settings" ? "active" : ""}
          onClick={() => setActiveTab("settings")}
        >
          <IconBell size={18} />
          매장·알림 설정
        </button>
      </div>

      {activeTab === "editor" && (
        <form className="sale-editor" onSubmit={save} role="tabpanel">
          <div className="section-title">
            <div>
              <p className="eyebrow">SALE CONTENT</p>
              <h3>{draft.id ? "세일 원문 편집" : "새 세일 원문"}</h3>
            </div>
            <button type="button" className="ghost" disabled={busy || !canManage} onClick={startNewDraft}>
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
          </div>

          <div className="sale-file-upload">
            <div>
              <strong>TXT 파일에서 불러오기</strong>
              <span>UTF-8 또는 EUC-KR 형식의 파일을 지원합니다.</span>
            </div>
            <label className={`sale-file-button${!canManage ? " disabled" : ""}`}>
              <IconFileUpload size={18} />
              TXT 파일 선택
              <input
                className="sale-file-input"
                type="file"
                accept=".txt,text/plain"
                disabled={!canManage}
                onChange={handleFile}
              />
            </label>
            {fileName && <span className="sale-file-name">{fileName}</span>}
          </div>

          <label className="sale-content-field">
            세일 원문
            <textarea
              className="sale-content-textarea"
              maxLength={MAX_TEXT_LENGTH}
              placeholder={"★한라마트 특급세일★\n\n○야채\n- 파프리카 3개  3,980\n- 백오이 5개  3,980\n\n○청과\n- 딸기 특 1팩  7,900"}
              value={draft.raw_text}
              onChange={(event) => {
                setDraft({ ...draft, raw_text: normalizeText(event.target.value) });
                setFileName("");
              }}
              disabled={!canManage}
            />
            <span className="sale-content-meta">
              앱에는 입력한 기호와 줄바꿈을 그대로 전달합니다.
              <span>{draft.raw_text.length.toLocaleString()} / {MAX_TEXT_LENGTH.toLocaleString()}자</span>
            </span>
          </label>

          <div className="form-actions split-actions">
            <span className="sale-save-note">게시 전에는 초안으로 저장됩니다.</span>
            <div>
              <button type="submit" className="ghost" disabled={busy || !canManage || !draft.raw_text.trim()}>
                <IconDeviceFloppy size={17} />
                초안 저장
              </button>
              <button
                type="button"
                disabled={busy || !canManage || !draft.id || !draft.raw_text.trim()}
                onClick={publish}
              >
                <IconSend size={17} />
                게시 및 알림
              </button>
            </div>
          </div>
        </form>
      )}

      {activeTab === "history" && (
        <section className="subpanel sale-history-panel" role="tabpanel">
          <div className="section-title">
            <div>
              <p className="eyebrow">PUBLISH HISTORY</p>
              <h3>게시 이력</h3>
            </div>
            <button type="button" disabled={!canManage} onClick={startNewDraft}>
              <IconPlus size={17} />
              새 글 작성
            </button>
          </div>
          <p className="sale-panel-help">초안과 게시된 세일 공지를 날짜순으로 확인하고 다시 편집할 수 있습니다.</p>
          <div className="table-wrap sale-history-board">
            <table>
              <thead>
                <tr>
                  <th>번호</th>
                  <th>행사일</th>
                  <th>제목</th>
                  <th>상태</th>
                  <th>버전</th>
                  <th>알림</th>
                  <th>게시 시각</th>
                  <th>관리</th>
                </tr>
              </thead>
              <tbody>
                {batches.map((batch, index) => (
                  <tr key={batch.id}>
                    <td>{batches.length - index}</td>
                    <td>{batch.sale_date}</td>
                    <td className="sale-history-title">
                      <button type="button" onClick={() => edit(batch)}>{batch.title}</button>
                    </td>
                    <td><span className={`badge ${batch.status}`}>{statusLabels[batch.status]}</span></td>
                    <td>{batch.version || "-"}</td>
                    <td>{batch.push_delivery ? deliveryLabels[batch.push_delivery.status] : "-"}</td>
                    <td>{formatDateTime(batch.published_at)}</td>
                    <td>
                      <div className="sale-row-actions">
                        <button type="button" className="ghost" onClick={() => edit(batch)}>
                          <IconPencil size={15} />
                          열기
                        </button>
                        {canManage && batch.status === "draft" && (
                          <button
                            type="button"
                            className="icon-button danger"
                            aria-label={`${batch.title} 초안 삭제`}
                            disabled={busy}
                            onClick={() => void remove(batch)}
                          >
                            <IconTrash size={16} />
                          </button>
                        )}
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            {!batches.length && <div className="empty">저장된 세일 공지가 없습니다.</div>}
          </div>
        </section>
      )}

      {activeTab === "settings" && (
        <form className="subpanel sale-settings" onSubmit={saveSettings} role="tabpanel">
          <div className="section-title">
            <div>
              <p className="eyebrow">STORE & PUSH</p>
              <h3>매장·알림 설정</h3>
            </div>
            <IconBell size={20} />
          </div>
          <label>매장 ID<input value={settings.store_id} onChange={(event) => setSettings({ ...settings, store_id: event.target.value })} disabled={!canManage} /></label>
          <label>매장 이름<input value={settings.store_name} onChange={(event) => setSettings({ ...settings, store_name: event.target.value })} disabled={!canManage} /></label>
          <label>FCM 주제<input value={settings.fcm_topic} onChange={(event) => setSettings({ ...settings, fcm_topic: event.target.value })} disabled={!canManage} /></label>
          <label>서비스 계정 파일 경로<input placeholder="/run/secrets/firebase.json" value={settings.fcm_service_account_file} onChange={(event) => setSettings({ ...settings, fcm_service_account_file: event.target.value })} disabled={!canManage} /></label>
          <label className="switch-label">
            <input type="checkbox" checked={settings.fcm_enabled} onChange={(event) => setSettings({ ...settings, fcm_enabled: event.target.checked })} disabled={!canManage} />
            FCM 푸시 사용
          </label>
          <button disabled={busy || !canManage}>설정 저장</button>
        </form>
      )}
    </div>
  );
}
