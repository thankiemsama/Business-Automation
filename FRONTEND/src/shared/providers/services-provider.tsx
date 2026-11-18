"use client";
import { createContext, useContext, useState, type ReactNode } from "react";
import { WorkspaceRepository } from "../api/workspace";
import { PlatformRepository } from "../api/platform";
import { FetchHttpClient, type HttpClient } from "../api/http-client";
import {
  ApiResourceRepository,
  AuthRepository,
  ConversationRepository,
  DashboardRepository,
  type ResourceRepository,
} from "../api/repositories";
import type { ResourceKind, ResourceMap } from "../domain/models";

export function createServices(http: HttpClient) {
  const conversations = new ConversationRepository(http);
  const resources: { [K in ResourceKind]: ResourceRepository<ResourceMap[K]> } =
    {
      customers: new ApiResourceRepository(http, "customers"),
      conversations,
      tickets: new ApiResourceRepository(http, "tickets"),
      knowledge: new ApiResourceRepository(http, "knowledge"),
      automations: new ApiResourceRepository(http, "automations"),
    };
  return {
    auth: new AuthRepository(http),
    workspace: new WorkspaceRepository(http),
    platform: new PlatformRepository(http),
    dashboard: new DashboardRepository(http),
    ...resources,
    conversations,
    resources,
  };
}
const ServicesContext = createContext<ReturnType<typeof createServices> | null>(
  null,
);
export function ServicesProvider({ children }: { children: ReactNode }) {
  const [services] = useState(() => createServices(new FetchHttpClient()));
  return <ServicesContext value={services}>{children}</ServicesContext>;
}
export function useServices() {
  const services = useContext(ServicesContext);
  if (!services) throw new Error("ServicesProvider is required");
  return services;
}
