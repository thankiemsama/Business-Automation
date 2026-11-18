import { AccountForm } from "@/modules/auth/account-form";

export default function SetupPasswordPage() {
  return (
    <main>
      <AccountForm action="auth/redeem" />
    </main>
  );
}
