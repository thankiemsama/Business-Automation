"use client";

import { useRef, useState } from "react";
import { FetchHttpClient, errorMessage } from "@/shared/api/http-client";
import { useQuery } from "@/shared/hooks/use-query";
import { useUser } from "@/shared/providers/workspace-provider";
import { ErrorState, Loading, PageHeading } from "@/shared/ui/primitives";
import { RelationSelect } from "../resources/relation-select";

type Line = { role: "agent" | "customer"; text: string };
type Action = "resolve_ticket" | "handoff";
type Scenario = { id: string; title: string };
type Session = {
  id: string;
  mode: "sparring" | "certification";
  title: string;
  status: string;
  lines?: Line[];
  assessments?: {
    score: number;
    coaching: string;
    action: Action;
    expected: Action;
    passed: boolean;
    baseline?: number;
    regression?: boolean;
  }[];
};

type Overview = {
  eval_sources?: { conversation_id: string; kind: string; at: string }[];
  enabled: boolean;
  scenarios: Scenario[];
  sessions: Session[];
  certified_until?: string;
};

const api = new FetchHttpClient();
const path = "ai/sparring";
const load = (signal: AbortSignal) => api.request<Overview>(path, { signal });
const actionLabels: Record<Action, string> = {
  resolve_ticket: "Giải quyết phiếu",
  handoff: "Chuyển người xử lý",
};
const evalSourceLabels: Record<string, string> = {
  qa: "QA",
  triage: "Phân loại AI",
  trace: "Dấu vết agent",
};
const statusLabels: Record<string, string> = {
  active: "Đang luyện",
  abandoned: "Đã kết thúc",
  queued: "Đang chờ chấm",
  completed: "Đã chấm",
  certified: "Đã cấp chứng chỉ",
  not_certified: "Chưa đạt chứng chỉ",
  failed: "Chấm điểm thất bại",
};
const date = new Intl.DateTimeFormat("vi-VN", {
  dateStyle: "medium",
  timeZone: "Asia/Ho_Chi_Minh",
});

function post<T>(route: string, payload?: object) {
  return api.request<T>(`${path}/${route}`, {
    method: "POST",
    ...(payload ? { body: JSON.stringify(payload) } : {}),
  });
}

function ScenarioBank({
  scenarios,
  sources,
  onChange,
}: {
  scenarios: Scenario[];
  sources: NonNullable<Overview["eval_sources"]>;
  onChange: () => void;
}) {
  const [source, setSource] = useState("");
  const [lines, setLines] = useState<Line[]>([]);
  const [approved, setApproved] = useState(false);
  const [busy, setBusy] = useState(false);
  const [feedback, setFeedback] = useState("");
  const [error, setError] = useState("");

  async function run(task: () => Promise<void>) {
    setBusy(true);
    setError("");
    setFeedback("");
    try {
      await task();
    } catch (failure) {
      setError(errorMessage(failure));
    } finally {
      setBusy(false);
    }
  }

  return (
    <details className="panel team-form form-stack">
      <summary>Ngân hàng kịch bản</summary>
      {!!sources.length && (
        <label>
          Nguồn gợi ý gần đây
          <select
            value={
              sources.some((item) => item.conversation_id === source)
                ? source
                : ""
            }
            disabled={busy}
            onChange={(event) => {
              setSource(event.target.value);
              setLines([]);
              setApproved(false);
            }}
          >
            <option value="">
              Chọn hội thoại từ QA, phân loại AI hoặc dấu vết agent
            </option>
            {sources.map((item) => (
              <option key={item.conversation_id} value={item.conversation_id}>
                {evalSourceLabels[item.kind] ?? item.kind} ·{" "}
                {date.format(new Date(item.at))} · #
                {item.conversation_id.slice(0, 8)}
              </option>
            ))}
          </select>
        </label>
      )}
      <label htmlFor="sparring-source">Hội thoại nguồn</label>
      <RelationSelect
        id="sparring-source"
        kind="conversations"
        value={source}
        disabled={busy}
        onChange={(value) => {
          setSource(value);
          setLines([]);
          setApproved(false);
        }}
      />
      <button
        type="button"
        className="button secondary"
        disabled={!source || busy}
        onClick={() =>
          void run(async () => {
            const preview = await api.request<Line[]>(
              `${path}/preview/${encodeURIComponent(source)}`,
            );
            setLines(preview);
            setApproved(false);
            if (preview.length < 2)
              setFeedback(
                "Hội thoại cần có cả lời khách và phản hồi nhân viên.",
              );
          })
        }
      >
        Tạo bản xem trước đã che thông tin
      </button>
      {lines.length > 1 && (
        <form
          className="form-stack"
          onSubmit={(event) => {
            event.preventDefault();
            const form = event.currentTarget;
            const data = new FormData(form);
            void run(async () => {
              await post("scenarios", {
                conversation_id: source,
                title: String(data.get("title") ?? ""),
                expected_action: String(data.get("expected_action") ?? ""),
                lines: lines.filter((line) => line.text.trim()),
                anonymized: approved,
              });
              form.reset();
              setLines([]);
              setApproved(false);
              setFeedback("Đã lưu kịch bản.");
              onChange();
            });
          }}
        >
          <label>
            Tiêu đề kịch bản
            <input
              name="title"
              maxLength={100}
              required
              disabled={busy}
              onChange={() => setApproved(false)}
            />
          </label>
          <label>
            Quyết định chuẩn
            <select name="expected_action" disabled={busy}>
              <option value="handoff">Chuyển người xử lý</option>
              <option value="resolve_ticket">Giải quyết phiếu</option>
            </select>
          </label>
          <p>
            Xóa tên, địa chỉ, mã hồ sơ và chi tiết có thể nhận dạng còn sót. Để
            trống một lượt nếu muốn bỏ lượt đó.
          </p>
          {lines.map((line, index) => (
            <label key={index}>
              {line.role === "customer" ? "Khách hàng" : "Nhân viên"} ·{" "}
              {index + 1}
              <textarea
                value={line.text}
                maxLength={600}
                disabled={busy}
                onChange={(event) => {
                  setLines((current) =>
                    current.map((item, i) =>
                      i === index
                        ? { ...item, text: event.target.value }
                        : item,
                    ),
                  );
                  setApproved(false);
                }}
              />
            </label>
          ))}
          <label className="check-row">
            <input
              type="checkbox"
              checked={approved}
              disabled={busy}
              onChange={(event) => setApproved(event.target.checked)}
            />
            Tôi đã rà soát tiêu đề và các lượt thoại để loại thông tin nhận
            dạng.
          </label>
          <button className="button primary" disabled={busy || !approved}>
            Lưu kịch bản
          </button>
        </form>
      )}
      <ul>
        {scenarios.map((item) => (
          <li key={item.id} className="integration-item">
            <span>{item.title}</span>
            <button
              type="button"
              className="text-link"
              disabled={busy}
              onClick={() =>
                void run(async () => {
                  await post(
                    `scenarios/${encodeURIComponent(item.id)}/archive`,
                  );
                  setFeedback("Đã lưu trữ kịch bản.");
                  onChange();
                })
              }
            >
              Lưu trữ
            </button>
          </li>
        ))}
      </ul>
      {feedback && <p role="status">{feedback}</p>}
      {error && <p role="alert">{error}</p>}
    </details>
  );
}

export function SparringPage() {
  const { role } = useUser();
  const overview = useQuery(load, undefined, 8000);
  const [scenarioID, setScenarioID] = useState("");
  const [answer, setAnswer] = useState("");
  const [action, setAction] = useState<Action>("handoff");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const request = useRef<{ key: string; id: string } | null>(null);
  const data = overview.data;
  const active = data?.sessions.find(
    (session) => session.mode === "sparring" && session.status === "active",
  );

  async function run(task: () => Promise<unknown>) {
    setBusy(true);
    setError("");
    try {
      await task();
      request.current = null;
      overview.reload();
    } catch (failure) {
      setError(errorMessage(failure));
    } finally {
      setBusy(false);
    }
  }

  function requestID(key: string) {
    if (request.current?.key !== key) {
      request.current = { key, id: crypto.randomUUID() };
    }
    return request.current.id;
  }

  const disabled = busy || overview.loading;
  return (
    <>
      <PageHeading
        title="AI Sparring"
        description="Luyện đối thoại với khách hàng mô phỏng và nhận coaching sau mỗi phiên."
      />
      {overview.error && (
        <ErrorState message={overview.error} retry={overview.reload} />
      )}
      {!data && !overview.error && <Loading />}
      {data && !data.enabled && (
        <p>
          Doanh nghiệp cần cấu hình OpenAI và bật AI Sparring trong Quản trị.
        </p>
      )}
      {data?.enabled && (
        <div className="form-stack">
          {role === "owner" && (
            <ScenarioBank
              scenarios={data.scenarios}
              sources={data.eval_sources ?? []}
              onChange={overview.reload}
            />
          )}
          <section className="panel team-form form-stack">
            <h2>Chứng chỉ AI agent</h2>
            <p>
              {data.certified_until
                ? `Có hiệu lực đến ${date.format(new Date(data.certified_until))}.`
                : "Chưa có chứng chỉ hợp lệ. AI agent chưa được phép giải quyết phiếu."}
            </p>
            {role === "owner" && (
              <>
                <p>
                  AI agent cần đạt 80/100 ở cả tình huống giải quyết và tình
                  huống chuyển người xử lý.
                </p>
                <button
                  type="button"
                  className="button secondary"
                  disabled={
                    disabled ||
                    data.sessions.some(
                      (session) =>
                        session.mode === "certification" &&
                        session.status === "queued",
                    )
                  }
                  onClick={() =>
                    void run(() =>
                      post("sessions", {
                        certification: true,
                        request_id: requestID("certification"),
                      }),
                    )
                  }
                >
                  Kiểm định AI agent
                </button>
              </>
            )}
          </section>
          <section className="panel team-form form-stack">
            <h2>{active?.title ?? "Luyện tập 1v1"}</h2>
            {active ? (
              <>
                <ol aria-live="polite" className="form-stack">
                  {active.lines?.map((line, i) => (
                    <li key={i}>
                      <strong>
                        {line.role === "customer" ? "Khách mô phỏng" : "Bạn"}
                      </strong>
                      <p>{line.text}</p>
                    </li>
                  ))}
                </ol>
                <form
                  className="form-stack"
                  onSubmit={(event) => {
                    event.preventDefault();
                    const turn = active.lines?.length ?? 0;
                    void run(async () => {
                      await post(`sessions/${active.id}/turn`, {
                        text: answer,
                        turn,
                        request_id: requestID(`${active.id}:${turn}:${answer}`),
                      });
                      setAnswer("");
                    });
                  }}
                >
                  <label>
                    Phản hồi của bạn
                    <textarea
                      value={answer}
                      maxLength={1000}
                      onChange={(event) => setAnswer(event.target.value)}
                      disabled={disabled || (active.lines?.length ?? 0) >= 17}
                    />
                  </label>
                  <button
                    className="button primary"
                    disabled={
                      disabled ||
                      !answer.trim() ||
                      (active.lines?.length ?? 0) >= 17
                    }
                  >
                    Gửi phản hồi
                  </button>
                </form>
                <label>
                  Quyết định kết thúc
                  <select
                    value={action}
                    onChange={(event) =>
                      setAction(event.target.value as Action)
                    }
                    disabled={disabled}
                  >
                    {Object.entries(actionLabels).map(([key, title]) => (
                      <option value={key} key={key}>
                        {title}
                      </option>
                    ))}
                  </select>
                </label>
                <div className="toolbar-controls">
                  <button
                    type="button"
                    className="button secondary"
                    disabled={disabled || (active.lines?.length ?? 0) < 3}
                    onClick={() =>
                      void run(() =>
                        post(`sessions/${active.id}/end`, { action }),
                      )
                    }
                  >
                    Nộp bài
                  </button>
                  <button
                    type="button"
                    className="text-link"
                    disabled={disabled}
                    onClick={() =>
                      void run(() =>
                        post(`sessions/${active.id}/end`, {
                          action: "abandon",
                        }),
                      )
                    }
                  >
                    Bỏ phiên
                  </button>
                </div>
              </>
            ) : (
              <>
                <label>
                  Kịch bản
                  <select
                    value={scenarioID}
                    disabled={disabled}
                    onChange={(event) => setScenarioID(event.target.value)}
                  >
                    <option value="">Chọn kịch bản</option>
                    {data.scenarios.map((item) => (
                      <option key={item.id} value={item.id}>
                        {item.title}
                      </option>
                    ))}
                  </select>
                </label>
                <button
                  type="button"
                  className="button primary"
                  disabled={disabled || !scenarioID}
                  onClick={() =>
                    void run(() =>
                      post("sessions", {
                        scenario_id: scenarioID,
                        request_id: requestID(scenarioID),
                      }),
                    )
                  }
                >
                  Bắt đầu luyện
                </button>
              </>
            )}
            {error && <p role="alert">{error}</p>}
          </section>
          <section className="panel team-form form-stack">
            <h2>Phiên gần đây</h2>
            {data.sessions.length === 0 && (
              <p>Chưa có phiên luyện tập hoặc kiểm định.</p>
            )}
            {data.sessions.map((session) => (
              <details key={session.id}>
                <summary>
                  {session.title} ·{" "}
                  {statusLabels[session.status] ?? session.status}
                </summary>
                {session.assessments?.map((result, i) => (
                  <div key={i}>
                    <p>
                      <strong>
                        {result.score}/100 ·{" "}
                        {result.passed ? "Đạt" : "Chưa đạt"}
                      </strong>
                    </p>
                    <p>
                      Đã chọn {actionLabels[result.action]}. Quyết định chuẩn là{" "}
                      {actionLabels[result.expected]}.
                    </p>
                    {result.baseline != null && (
                      <p>
                        Mốc đối chiếu {result.baseline}/100
                        {result.regression ? " · Giảm quá 5 điểm" : ""}.
                      </p>
                    )}
                    <p>{result.coaching}</p>
                  </div>
                ))}
              </details>
            ))}
          </section>
        </div>
      )}
    </>
  );
}
