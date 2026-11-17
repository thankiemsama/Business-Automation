"use client";
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useState,
  type ReactNode,
} from "react";
import { useServices, ServicesProvider } from "./services-provider";
import { useQuery } from "../hooks/use-query";
import { ApiError } from "../api/http-client";
import type { User } from "../domain/models";
import { AuthScreen } from "@/modules/auth/auth-screen";
import { ErrorState, Loading } from "../ui/primitives";
import { AppShell } from "@/shared/ui/app-shell";
const UserContext = createContext<User | null>(null);
export function useUser() {
  const user = useContext(UserContext);
  if (!user) throw new Error("Workspace session is required");
  return user;
}
function SessionGate({ children }: { children: ReactNode }) {
  const { auth } = useServices();
  const [signedIn, setSignedIn] = useState<User | null>(null);
  const [expired, setExpired] = useState(false);
  const load = useCallback(async () => {
    try {
      return await auth.me();
    } catch (error) {
      if (error instanceof ApiError && error.status === 401) return null;
      throw error;
    }
  }, [auth]);
  const session = useQuery(load);
  useEffect(() => {
    const expire = () => {
      setExpired(true);
      setSignedIn(null);
    };
    window.addEventListener("session-expired", expire);
    return () => window.removeEventListener("session-expired", expire);
  }, []);
  const user = signedIn || (!expired ? session.data : null);
  if (session.loading && !signedIn)
    return (
      <div className="boot-screen">
        <Loading />
      </div>
    );
  if (session.error && !signedIn && !expired)
    return (
      <div className="boot-screen">
        <ErrorState message={session.error} retry={session.reload} />
      </div>
    );
  if (!user)
    return (
      <AuthScreen
        onSuccess={(user) => {
          setSignedIn(user);
          setExpired(false);
        }}
      />
    );
  async function logout() {
    await auth.logout();
    setSignedIn(null);
    setExpired(true);
  }
  return (
    <UserContext value={user}>
      <AppShell user={user} onLogout={logout}>
        {children}
      </AppShell>
    </UserContext>
  );
}
export function WorkspaceProvider({ children }: { children: ReactNode }) {
  return (
    <ServicesProvider>
      <SessionGate>{children}</SessionGate>
    </ServicesProvider>
  );
}
