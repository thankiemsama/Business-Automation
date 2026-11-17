import { customers } from "../customers/definition";
import { conversations } from "../conversations/definition";
import { tickets } from "../tickets/definition";
import { knowledge } from "../knowledge/definition";
import { automations } from "../automations/definition";
import type { ResourceKind } from "@/shared/domain/models";
import type { ModuleDefinition } from "./module-definition";
export const resourceModules: { [K in ResourceKind]: ModuleDefinition<K> } = {
  customers,
  conversations,
  tickets,
  knowledge,
  automations,
};
