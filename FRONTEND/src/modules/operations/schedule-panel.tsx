"use client";
import { useState, type FormEvent } from "react";
import { useQuery } from "@/shared/hooks/use-query";
import {
  loadSchedules,
  removeOperation,
  saveOperation,
  type Schedule,
} from "@/shared/api/operations";
import {
  useActiveMembers,
  useOperationMutation,
  OperationFeedback,
} from "./operation-panel-state";

const newSchedule: Schedule = {
  name: "",
  enabled: true,
  recipient_id: "",
  frequency: "weekly",
  hour: 8,
  day: 1,
};

export function SchedulePanel() {
  const { members, people } = useActiveMembers();
  const schedules = useQuery(loadSchedules);
  const [schedule, setSchedule] = useState(newSchedule);
  const { busy, error, notice, mutate } = useOperationMutation();
  function saveSchedule(event: FormEvent) {
    event.preventDefault();
    void mutate(
      async () => {
        await saveOperation("schedules", schedule);
        setSchedule(newSchedule);
      },
      schedules.reload,
      "Đã lưu lịch gửi.",
    );
  }

  return (
    <section id="report-schedules" className="panel team-form form-stack">
      <h2>Lịch gửi báo cáo</h2>
      <OperationFeedback
        error={error || members.error || schedules.error}
        notice={notice}
      />
      {schedules.data && !schedules.data.email_enabled && (
        <p className="subtle-note">
          Chưa cấu hình Resend. Có thể lưu lịch trước.
        </p>
      )}
      {schedules.data?.items.map((item) => (
        <div key={item.id} className="integration-item">
          <strong>{item.name}</strong>
          <span>
            {item.enabled && item.next_at
              ? new Date(item.next_at).toLocaleString("vi-VN", {
                  timeZone: "Asia/Ho_Chi_Minh",
                })
              : "Đã tắt"}
          </span>
          <button
            className="text-link"
            type="button"
            onClick={() => setSchedule(item)}
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
                  await removeOperation("schedules", item.id!);
                  if (schedule.id === item.id) setSchedule(newSchedule);
                },
                schedules.reload,
                "Đã xóa lịch gửi.",
              )
            }
          >
            Xóa
          </button>
        </div>
      ))}
      <form className="form-stack" onSubmit={saveSchedule}>
        <label>
          Tên lịch
          <input
            maxLength={100}
            required
            value={schedule.name}
            onChange={(event) =>
              setSchedule({ ...schedule, name: event.target.value })
            }
          />
        </label>
        <label>
          Người nhận
          <select
            required
            value={schedule.recipient_id}
            onChange={(event) =>
              setSchedule({ ...schedule, recipient_id: event.target.value })
            }
          >
            <option value="">Chọn thành viên</option>
            {people.map((user) => (
              <option key={user.id} value={user.id}>
                {user.name} · {user.email}
              </option>
            ))}
          </select>
        </label>
        <label>
          Chu kỳ
          <select
            value={schedule.frequency}
            onChange={(event) =>
              setSchedule({
                ...schedule,
                frequency: event.target.value as Schedule["frequency"],
                day: 1,
              })
            }
          >
            <option value="weekly">Hàng tuần</option>
            <option value="monthly">Hàng tháng</option>
          </select>
        </label>
        {schedule.frequency === "weekly" ? (
          <label>
            Ngày gửi
            <select
              value={schedule.day}
              onChange={(event) =>
                setSchedule({ ...schedule, day: Number(event.target.value) })
              }
            >
              {[
                "Thứ Hai",
                "Thứ Ba",
                "Thứ Tư",
                "Thứ Năm",
                "Thứ Sáu",
                "Thứ Bảy",
                "Chủ Nhật",
              ].map((day, index) => (
                <option key={day} value={index + 1}>
                  {day}
                </option>
              ))}
            </select>
          </label>
        ) : (
          <label>
            Ngày gửi (1–28)
            <input
              required
              type="number"
              min={1}
              max={28}
              value={schedule.day}
              onChange={(event) =>
                setSchedule({ ...schedule, day: Number(event.target.value) })
              }
            />
          </label>
        )}
        <label>
          Giờ gửi (giờ Việt Nam)
          <input
            required
            type="number"
            min={0}
            max={23}
            value={schedule.hour}
            onChange={(event) =>
              setSchedule({ ...schedule, hour: Number(event.target.value) })
            }
          />
        </label>
        <label>
          <input
            type="checkbox"
            checked={schedule.enabled}
            onChange={(event) =>
              setSchedule({ ...schedule, enabled: event.target.checked })
            }
          />{" "}
          Bật lịch gửi
        </label>
        <div className="toolbar-controls">
          <button className="button primary" disabled={busy}>
            {schedule.id ? "Lưu thay đổi" : "Tạo lịch gửi"}
          </button>
          {schedule.id && (
            <button
              type="button"
              className="button secondary"
              onClick={() => setSchedule(newSchedule)}
            >
              Hủy chỉnh sửa
            </button>
          )}
        </div>
      </form>
    </section>
  );
}
