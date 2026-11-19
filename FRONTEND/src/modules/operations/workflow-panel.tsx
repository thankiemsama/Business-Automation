"use client";
import { useCallback, useState, type FormEvent } from "react";
import { useServices } from "@/shared/providers/services-provider";
import { useQuery } from "@/shared/hooks/use-query";
import {
  loadWorkflows,
  migrateWorkflow,
  removeOperation,
  saveOperation,
  type Action,
  type Conditions,
  type Workflow,
} from "@/shared/api/operations";
import type { User } from "@/shared/domain/models";
import {
  useActiveMembers,
  useOperationMutation,
  OperationFeedback,
} from "./operation-panel-state";

const noConditions: Conditions = {
  channel: "",
  priority: "",
  status: "",
  contains: "",
};
const newWorkflow: Workflow = {
  name: "",
  enabled: true,
  trigger: "ticket_created",
  idle_days: 3,
  conditions: noConditions,
  actions: [{ type: "notify_owner" }],
};
const triggers = {
  ticket_created: "Phiếu mới",
  message_received: "Tin nhắn khách hàng",
  ticket_idle: "Phiếu không hoạt động",
  sla_breached: "Phiếu vi phạm SLA",
};
const actions = {
  notify_owner: "Thông báo cho chủ doanh nghiệp",
  email_owner: "Gửi email chủ doanh nghiệp",
  assign_owner: "Chuyển cho chủ doanh nghiệp",
  assign: "Đổi người phụ trách",
  create_ticket: "Tạo phiếu hỗ trợ",
  resolve_ticket: "Giải quyết phiếu",
};
const channels = {
  web: "Website",
  email: "Email",
  phone: "Điện thoại",
  zalo: "Zalo",
  messenger: "Messenger",
  whatsapp: "WhatsApp",
  sms: "SMS",
  instagram: "Instagram",
};
const priorities = {
  low: "Thấp",
  normal: "Bình thường",
  high: "Cao",
  urgent: "Khẩn cấp",
};

export function WorkflowPanel() {
  const { automations } = useServices();
  const { members, people } = useActiveMembers();
  const workflows = useQuery(loadWorkflows);
  const legacyLoad = useCallback(
    (signal: AbortSignal) => automations.list({ limit: 100 }, signal),
    [automations],
  );
  const legacy = useQuery(legacyLoad);

  const [workflow, setWorkflow] = useState(newWorkflow);
  const { busy, error, notice, mutate } = useOperationMutation();
  function saveWorkflow(event: FormEvent) {
    event.preventDefault();
    void mutate(
      async () => {
        await saveOperation("workflows", workflow);
        setWorkflow(newWorkflow);
      },
      workflows.reload,
      "Đã lưu workflow.",
    );
  }

  return (
    <section className="panel team-form form-stack">
      <h2>Workflow nhiều bước</h2>
      <OperationFeedback
        error={error || members.error || workflows.error || legacy.error}
        notice={notice}
      />
      {legacy.data?.items.some((item) => item.enabled) && (
        <details className="operations-fields">
          <summary>Chuyển quy tắc từ khóa đang chạy</summary>
          {legacy.data.items
            .filter((item) => item.enabled)
            .map((item) => (
              <div key={item.id} className="integration-item">
                <strong>{item.name}</strong>
                <button
                  type="button"
                  className="text-link"
                  disabled={busy}
                  onClick={() =>
                    void mutate(
                      async () => {
                        const rule = await migrateWorkflow(item.id);
                        setWorkflow(rule);
                        legacy.reload();
                      },
                      workflows.reload,
                      "Đã chuyển sang workflow và tắt quy tắc cũ.",
                    )
                  }
                >
                  Chuyển sang workflow
                </button>
              </div>
            ))}
        </details>
      )}
      {workflows.data?.map((rule) => (
        <div key={rule.id} className="integration-item">
          <strong>{rule.name}</strong>
          <span>
            {triggers[rule.trigger as keyof typeof triggers]} ·{" "}
            {rule.enabled ? "Đang bật" : "Đã tắt"}
          </span>
          <button
            className="text-link"
            type="button"
            onClick={() => setWorkflow(rule)}
          >
            Chỉnh sửa
          </button>
          <button
            className="text-link"
            type="button"
            disabled={busy}
            onClick={() =>
              void mutate(
                async () => {
                  await removeOperation("workflows", rule.id!);
                  if (workflow.id === rule.id) setWorkflow(newWorkflow);
                },
                workflows.reload,
                "Đã xóa workflow.",
              )
            }
          >
            Xóa
          </button>
        </div>
      ))}
      <form className="form-stack" onSubmit={saveWorkflow}>
        <label>
          Tên workflow
          <input
            maxLength={100}
            required
            value={workflow.name}
            onChange={(event) =>
              setWorkflow({ ...workflow, name: event.target.value })
            }
          />
        </label>
        <label>
          Sự kiện kích hoạt
          <select
            value={workflow.trigger}
            onChange={(event) =>
              setWorkflow({ ...workflow, trigger: event.target.value })
            }
          >
            {Object.entries(triggers).map(([value, label]) => (
              <option key={value} value={value}>
                {label}
              </option>
            ))}
          </select>
        </label>
        {workflow.trigger === "ticket_idle" && (
          <label>
            Số ngày không cập nhật
            <input
              required
              type="number"
              min={1}
              max={365}
              value={workflow.idle_days || 3}
              onChange={(event) =>
                setWorkflow({
                  ...workflow,
                  idle_days: Number(event.target.value),
                })
              }
            />
          </label>
        )}
        <ConditionFields
          title="Điều kiện kích hoạt (khớp tất cả)"
          value={workflow.conditions}
          trigger={workflow.trigger}
          onChange={(conditions) => setWorkflow({ ...workflow, conditions })}
        />
        <ActionFields
          title="Hành động theo thứ tự"
          items={workflow.actions}
          trigger={workflow.trigger}
          people={people}
          onChange={(next) => setWorkflow({ ...workflow, actions: next })}
        />
        <label>
          <input
            type="checkbox"
            checked={Boolean(workflow.branch)}
            onChange={(event) =>
              setWorkflow({
                ...workflow,
                branch: event.target.checked
                  ? { conditions: noConditions, then: [], otherwise: [] }
                  : undefined,
              })
            }
          />{" "}
          Thêm nhánh điều kiện
        </label>
        {workflow.branch && (
          <>
            <ConditionFields
              title="Điều kiện của nhánh"
              value={workflow.branch.conditions}
              trigger={workflow.trigger}
              onChange={(conditions) =>
                setWorkflow({
                  ...workflow,
                  branch: { ...workflow.branch!, conditions },
                })
              }
            />
            <ActionFields
              title="Nếu đúng"
              items={workflow.branch.then}
              trigger={workflow.trigger}
              people={people}
              onChange={(then) =>
                setWorkflow({
                  ...workflow,
                  branch: { ...workflow.branch!, then },
                })
              }
            />
            <ActionFields
              title="Nếu sai"
              items={workflow.branch.otherwise}
              trigger={workflow.trigger}
              people={people}
              onChange={(otherwise) =>
                setWorkflow({
                  ...workflow,
                  branch: { ...workflow.branch!, otherwise },
                })
              }
            />
          </>
        )}
        <label>
          <input
            type="checkbox"
            checked={workflow.enabled}
            onChange={(event) =>
              setWorkflow({ ...workflow, enabled: event.target.checked })
            }
          />{" "}
          Bật workflow
        </label>
        <div className="toolbar-controls">
          <button className="button primary" disabled={busy}>
            {workflow.id ? "Lưu thay đổi" : "Tạo workflow"}
          </button>
          {workflow.id && (
            <button
              type="button"
              className="button secondary"
              onClick={() => setWorkflow(newWorkflow)}
            >
              Hủy chỉnh sửa
            </button>
          )}
        </div>
      </form>
    </section>
  );
}

function ConditionFields({
  title,
  value,
  trigger,
  onChange,
}: {
  title: string;
  value: Conditions;
  trigger: string;
  onChange: (value: Conditions) => void;
}) {
  const statuses =
    trigger === "message_received"
      ? { open: "Đang mở", closed: "Đã đóng" }
      : {
          open: "Đang mở",
          in_progress: "Đang xử lý",
          resolved: "Đã giải quyết",
        };
  return (
    <fieldset className="form-stack operations-fields">
      <legend>{title}</legend>
      {value.keywords_any ? (
        <label>
          Khớp một trong các từ khóa
          <input
            value={value.keywords_any.join(",")}
            onChange={(event) =>
              onChange({
                ...value,
                keywords_any: event.target.value
                  ? event.target.value.split(",")
                  : [],
              })
            }
          />
        </label>
      ) : null}
      {(
        [
          { key: "channel", label: "Kênh", options: channels },
          { key: "priority", label: "Ưu tiên", options: priorities },
          { key: "status", label: "Trạng thái", options: statuses },
        ] as const
      ).map(({ key, label, options }) => (
        <label key={key}>
          {label}
          <select
            value={value[key]}
            onChange={(event) =>
              onChange({ ...value, [key]: event.target.value })
            }
          >
            <option value="">Bất kỳ</option>
            {Object.entries(options).map(([id, label]) => (
              <option key={id} value={id}>
                {label}
              </option>
            ))}
          </select>
        </label>
      ))}
      <label>
        Nội dung chứa
        <input
          maxLength={120}
          value={value.contains}
          onChange={(event) =>
            onChange({ ...value, contains: event.target.value })
          }
        />
      </label>
    </fieldset>
  );
}

function ActionFields({
  title,
  items,
  trigger,
  people,
  onChange,
}: {
  title: string;
  items: Action[];
  trigger: string;
  people: User[];
  onChange: (actions: Action[]) => void;
}) {
  function set(index: number, action: Action) {
    onChange(items.map((item, i) => (i === index ? action : item)));
  }
  function move(index: number, step: number) {
    const next = [...items];
    [next[index], next[index + step]] = [next[index + step], next[index]];
    onChange(next);
  }
  return (
    <fieldset className="form-stack operations-fields">
      <legend>{title}</legend>
      {items.map((action, i) => (
        <div className="form-stack operation-action" key={i}>
          <label>
            Hành động {i + 1}
            <select
              value={action.type}
              onChange={(event) => set(i, { type: event.target.value })}
            >
              {Object.entries(actions)
                .filter(
                  ([value]) =>
                    value !== "resolve_ticket" ||
                    trigger !== "message_received",
                )
                .map(([value, text]) => (
                  <option key={value} value={value}>
                    {text}
                  </option>
                ))}
            </select>
          </label>
          {action.type === "assign" && (
            <label>
              Người phụ trách
              <select
                required
                value={action.value || ""}
                onChange={(event) =>
                  set(i, { ...action, value: event.target.value })
                }
              >
                <option value="">Chọn thành viên</option>
                {people.map((member) => (
                  <option key={member.id} value={member.id}>
                    {member.name}
                  </option>
                ))}
              </select>
            </label>
          )}
          {(action.type === "create_ticket" ||
            action.type === "notify_owner") && (
            <label>
              Tiêu đề tùy chọn
              <input
                maxLength={200}
                value={action.value || ""}
                onChange={(event) =>
                  set(i, { ...action, value: event.target.value })
                }
              />
            </label>
          )}
          <div className="toolbar-controls">
            <button
              type="button"
              className="text-link"
              disabled={i === 0}
              onClick={() => move(i, -1)}
              aria-label={`Đưa hành động ${i + 1} lên`}
            >
              Lên
            </button>
            <button
              type="button"
              className="text-link"
              disabled={i === items.length - 1}
              onClick={() => move(i, 1)}
              aria-label={`Đưa hành động ${i + 1} xuống`}
            >
              Xuống
            </button>
            <button
              type="button"
              className="text-link"
              onClick={() => onChange(items.filter((_, index) => i !== index))}
            >
              Bỏ hành động
            </button>
          </div>
        </div>
      ))}
      <button
        type="button"
        className="button secondary compact"
        disabled={items.length >= 12}
        onClick={() => onChange([...items, { type: "notify_owner" }])}
      >
        Thêm hành động
      </button>
    </fieldset>
  );
}
