"use client";
import Link from "next/link";
import { useCallback, useState } from "react";
import { useServices } from "@/shared/providers/services-provider";
import { useQuery } from "@/shared/hooks/use-query";
import {
  Badge,
  ErrorState,
  Loading,
  PageHeading,
} from "@/shared/ui/primitives";
import { ConversationThread } from "../conversations/conversation-thread";
import { ResourceEditor } from "./resource-editor";
import { AISummary, AITicketReview, AITicketSuggestion } from "../ai/assist";
import { TicketAgent } from "../ai/ticket-agent";
import { CustomValues } from "../admin/custom-values";
import { Assignment } from "../team/assignment";
import type { ResourceMap } from "@/shared/domain/models";
import customerStyles from "../customers/customer-profile.module.css";

export function ConversationDetail({ id }: { id: string }) {
  const { conversations } = useServices();
  const loader = useCallback(
    (signal: AbortSignal) => conversations.get(id, signal),
    [conversations, id],
  );
  const result = useQuery(loader, "workspace-refresh");
  const [editing, setEditing] = useState(false);
  const item = result.data;
  if (result.error)
    return <ErrorState message={result.error} retry={result.reload} />;
  if (!item) return <Loading />;
  return (
    <>
      <PageHeading
        title={item.subject || "Hội thoại"}
        eyebrow="HỘP HỘI THOẠI"
        description=""
      >
        <Link className="button secondary" href="/conversations">
          Về danh sách
        </Link>
      </PageHeading>
      <div className="panel workspace-detail-thread has-selection">
        <ConversationThread
          showDetails={false}
          conversation={item}
          onEdit={() => setEditing(true)}
          onChanged={result.reload}
          onAssigned={result.reload}
        />
      </div>
      <CustomValues kind="conversations" id={id} />
      {editing && (
        <ResourceEditor
          kind="conversations"
          repository={conversations}
          entity={item}
          onClose={() => setEditing(false)}
          onSaved={() => {
            setEditing(false);
            result.reload();
          }}
        />
      )}
    </>
  );
}

export function WorkspaceDetail<K extends "customers" | "tickets">({
  kind,
  id,
}: {
  kind: K;
  id: string;
}) {
  const { resources } = useServices();
  const repository = resources[kind];
  const loader = useCallback(
    (signal: AbortSignal) => repository.get(id, signal),
    [repository, id],
  );
  const result = useQuery<ResourceMap[K]>(loader, "workspace-refresh");
  const [editing, setEditing] = useState(false);
  const item = result.data;
  if (result.error)
    return <ErrorState message={result.error} retry={result.reload} />;
  if (!item) return <Loading />;
  const entity: ResourceMap["customers" | "tickets"] = item;
  const ticket = "status" in entity ? entity : null;
  const customer = "name" in entity ? entity : null;
  return (
    <>
      <PageHeading
        title={ticket?.subject || customer?.name || "Chi tiết"}
        eyebrow={ticket ? "PHIẾU HỖ TRỢ" : "KHÁCH HÀNG"}
        description={`#${id.slice(0, 8)}`}
      >
        <Link className="button secondary" href={`/${kind}`}>
          Về danh sách
        </Link>
        <button className="button primary" onClick={() => setEditing(true)}>
          Chỉnh sửa
        </button>
      </PageHeading>
      <CustomValues kind={kind} id={id} />
      <section
        className={
          customer
            ? `panel ${customerStyles.contact}`
            : "panel team-form form-stack"
        }
      >
        {ticket && (
          <>
            <div className="toolbar-controls">
              <Badge value={ticket.status} /> <Badge value={ticket.priority} />
            </div>
            <p>{ticket.content}</p>
            {ticket.sla_response_p90_minutes != null && (
              <p>
                Phản hồi dự báo (p50 / p90): {ticket.sla_response_p50_minutes} /{" "}
                {ticket.sla_response_p90_minutes} phút
              </p>
            )}
            {ticket.sla_resolution_p90_minutes != null && (
              <p>
                Xử lý dự báo (p50 / p90): {ticket.sla_resolution_p50_minutes} /{" "}
                {ticket.sla_resolution_p90_minutes} phút
              </p>
            )}
            {ticket.sla_risk_reason && (
              <p role="status">{ticket.sla_risk_reason}</p>
            )}
            <AISummary
              kind="tickets"
              id={ticket.id}
              saved={ticket.ai_summary}
            />
            {ticket.status === "resolved" ? (
              <AITicketReview id={ticket.id} />
            ) : (
              <AITicketSuggestion id={ticket.id} onApplied={result.reload} />
            )}
            <TicketAgent
              key={ticket.id}
              id={ticket.id}
              onApplied={result.reload}
            />
            {Boolean(ticket.tags?.length) && (
              <div className="keywords">
                {ticket.tags?.map((tag) => (
                  <span key={tag}>{tag}</span>
                ))}
              </div>
            )}
            {ticket.conversation_id && (
              <Link
                className="text-link"
                href={`/conversations/${ticket.conversation_id}`}
              >
                Mở hội thoại liên quan
              </Link>
            )}
            <Assignment
              assignee={ticket.assignee_id}
              save={async (agent, claim) => {
                await repository.assign(id, agent, claim);
                result.reload();
              }}
            />
          </>
        )}
        {customer && (
          <dl aria-label="Thông tin liên hệ">
            <div>
              <dt>Email</dt>
              <dd>
                {customer.email ? (
                  <a
                    href={`mailto:${encodeURIComponent(customer.email).replace(/%40/g, "@")}`}
                  >
                    {customer.email}
                  </a>
                ) : (
                  "Chưa cung cấp"
                )}
              </dd>
            </div>
            <div>
              <dt>Điện thoại</dt>
              <dd>{customer.phone || "Chưa cung cấp"}</dd>
            </div>
          </dl>
        )}
      </section>
      {editing && (
        <ResourceEditor
          kind={kind}
          repository={repository}
          entity={item}
          onClose={() => setEditing(false)}
          onSaved={() => {
            setEditing(false);
            result.reload();
          }}
        />
      )}
    </>
  );
}
