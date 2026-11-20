"use client";

import { useState } from "react";
import {
  loadSkillRouting,
  saveSkillRouting,
  type SkillGroup,
  type SkillRoute,
  type SkillRouting,
} from "@/shared/api/operations";
import { useQuery } from "@/shared/hooks/use-query";
import { ErrorState, Loading } from "@/shared/ui/primitives";
import layout from "@/shared/ui/workspace-settings.module.css";
import {
  OperationFeedback,
  useActiveMembers,
  useOperationMutation,
} from "./operation-panel-state";

const channels = {
  web: "Website",
  email: "Email",
  phone: "Điện thoại",
  zalo: "Zalo OA",
  whatsapp: "WhatsApp",
  sms: "SMS",
  messenger: "Messenger",
  instagram: "Instagram",
};
const priorities = {
  low: "Thấp",
  normal: "Bình thường",
  high: "Cao",
  urgent: "Khẩn cấp",
};

export function SkillRoutingPanel() {
  const loaded = useQuery(loadSkillRouting);
  const { members, people } = useActiveMembers();
  const [draft, setDraft] = useState<SkillRouting>();
  const action = useOperationMutation();
  if (loaded.error)
    return <ErrorState message={loaded.error} retry={loaded.reload} />;
  if (members.error)
    return <ErrorState message={members.error} retry={members.reload} />;
  const config = draft ?? loaded.data;
  if (!config || members.loading) return <Loading />;
  const groups = config.groups ?? [];
  const rules = config.rules ?? [];

  const changeGroup = (key: string, update: Partial<SkillGroup>) =>
    setDraft({
      ...config,
      groups: groups.map((group) =>
        group.key === key ? { ...group, ...update } : group,
      ),
    });
  const changeRule = (index: number, update: Partial<SkillRoute>) =>
    setDraft({
      ...config,
      rules: rules.map((rule, i) =>
        i === index ? { ...rule, ...update } : rule,
      ),
    });

  return (
    <form
      className={layout.workspace}
      onSubmit={(event) => {
        event.preventDefault();
        void action.mutate(
          async () => {
            setDraft(await saveSkillRouting(config));
          },
          loaded.reload,
          "Đã lưu cấu hình định tuyến.",
        );
      }}
    >
      <OperationFeedback error={action.error} notice={action.notice} />
      <fieldset
        className={`form-stack ${layout.workspace}`}
        disabled={action.busy}
      >
        <label className="check-row">
          <input
            type="checkbox"
            checked={config.enabled}
            onChange={(event) =>
              setDraft({ ...config, enabled: event.target.checked })
            }
          />
          Tự động phân công hội thoại mới
        </label>
        <div className={layout.split}>
          <section className={`panel ${layout.card}`}>
            <h3>Nhóm kỹ năng</h3>
            {!groups.length && (
              <p>Thêm nhóm và chọn thành viên có thể nhận hội thoại.</p>
            )}
            {groups.map((group, index) => (
              <fieldset
                className="operations-fields form-stack"
                key={group.key}
              >
                <legend>{group.name || `Nhóm ${index + 1}`}</legend>
                <label>
                  Tên nhóm
                  <input
                    required
                    maxLength={80}
                    value={group.name}
                    onChange={(event) =>
                      changeGroup(group.key, { name: event.target.value })
                    }
                  />
                </label>
                <label>
                  Số hội thoại đang xử lý tối đa mỗi người
                  <input
                    type="number"
                    min={1}
                    max={100}
                    required
                    value={group.capacity}
                    onChange={(event) =>
                      changeGroup(group.key, {
                        capacity: Number(event.target.value),
                      })
                    }
                  />
                </label>
                <label>
                  Thành viên
                  <select
                    multiple
                    required
                    value={group.members}
                    onChange={(event) =>
                      changeGroup(group.key, {
                        members: Array.from(
                          event.target.selectedOptions,
                          (item) => item.value,
                        ),
                      })
                    }
                  >
                    {people.map((member) => (
                      <option key={member.id} value={member.id}>
                        {member.name || member.email}
                      </option>
                    ))}
                  </select>
                </label>
                <button
                  type="button"
                  className="button secondary compact"
                  disabled={rules.some(
                    (rule) =>
                      rule.primary === group.key || rule.overflow === group.key,
                  )}
                  onClick={() =>
                    setDraft({
                      ...config,
                      groups: groups.filter((item) => item.key !== group.key),
                    })
                  }
                >
                  Xóa nhóm
                </button>
              </fieldset>
            ))}
            <button
              type="button"
              className="button secondary"
              disabled={groups.length >= 30}
              onClick={() =>
                setDraft({
                  ...config,
                  groups: [
                    ...groups,
                    {
                      key: Array.from(
                        crypto.getRandomValues(new Uint32Array(4)),
                        (value) => value.toString(16).padStart(8, "0"),
                      ).join(""),
                      name: "",
                      capacity: 5,
                      members: [],
                    },
                  ],
                })
              }
            >
              Thêm nhóm
            </button>
          </section>
          <section className={`panel ${layout.card}`}>
            <h3>Quy tắc phân công</h3>
            <p>
              Quy tắc được xét từ trên xuống. Đặt quy tắc có nhãn VIP phía trên
              quy tắc chung.
            </p>
            {rules.map((rule, index) => (
              <fieldset className="operations-fields form-stack" key={index}>
                <legend>Quy tắc {index + 1}</legend>
                <label>
                  Kênh
                  <select
                    value={rule.channel}
                    onChange={(event) =>
                      changeRule(index, { channel: event.target.value })
                    }
                  >
                    <option value="">Mọi kênh</option>
                    {Object.entries(channels).map(([key, label]) => (
                      <option key={key} value={key}>
                        {label}
                      </option>
                    ))}
                  </select>
                </label>
                <label>
                  Nhãn hội thoại hoặc khách hàng
                  <input
                    maxLength={40}
                    value={rule.tag}
                    onChange={(event) =>
                      changeRule(index, { tag: event.target.value })
                    }
                  />
                </label>
                <label>
                  Độ ưu tiên
                  <select
                    value={rule.priority}
                    onChange={(event) =>
                      changeRule(index, { priority: event.target.value })
                    }
                  >
                    <option value="">Mọi mức</option>
                    {Object.entries(priorities).map(([key, label]) => (
                      <option key={key} value={key}>
                        {label}
                      </option>
                    ))}
                  </select>
                </label>
                <label>
                  Nhóm chính
                  <select
                    required
                    value={rule.primary}
                    onChange={(event) =>
                      changeRule(index, {
                        primary: event.target.value,
                        overflow:
                          rule.overflow === event.target.value
                            ? ""
                            : rule.overflow,
                      })
                    }
                  >
                    <option value="">Chọn nhóm</option>
                    {groups.map((group) => (
                      <option key={group.key} value={group.key}>
                        {group.name}
                      </option>
                    ))}
                  </select>
                </label>
                <label>
                  Nhóm dự phòng
                  <select
                    value={rule.overflow}
                    onChange={(event) =>
                      changeRule(index, { overflow: event.target.value })
                    }
                  >
                    <option value="">
                      Để hội thoại chưa phân công nếu nhóm chính đầy
                    </option>
                    {groups
                      .filter((group) => group.key !== rule.primary)
                      .map((group) => (
                        <option key={group.key} value={group.key}>
                          {group.name}
                        </option>
                      ))}
                  </select>
                </label>
                <div className="toolbar-controls">
                  <button
                    type="button"
                    className="button secondary compact"
                    disabled={index === 0}
                    onClick={() => {
                      const ordered = [...rules];
                      [ordered[index - 1], ordered[index]] = [
                        ordered[index],
                        ordered[index - 1],
                      ];
                      setDraft({ ...config, rules: ordered });
                    }}
                  >
                    Đưa lên
                  </button>
                  <button
                    type="button"
                    className="button secondary compact"
                    onClick={() =>
                      setDraft({
                        ...config,
                        rules: rules.filter((_, i) => i !== index),
                      })
                    }
                  >
                    Xóa quy tắc
                  </button>
                </div>
              </fieldset>
            ))}
            <button
              type="button"
              className="button secondary"
              disabled={!groups.length || rules.length >= 50}
              onClick={() =>
                setDraft({
                  ...config,
                  rules: [
                    ...rules,
                    {
                      channel: "",
                      tag: "",
                      priority: "",
                      primary: groups[0].key,
                      overflow: "",
                    },
                  ],
                })
              }
            >
              Thêm quy tắc
            </button>
          </section>
        </div>
        <button className="button primary" disabled={action.busy || !draft}>
          Lưu định tuyến
        </button>
      </fieldset>
    </form>
  );
}
