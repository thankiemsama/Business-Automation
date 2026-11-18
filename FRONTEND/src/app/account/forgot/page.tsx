import { AccountForm } from "@/modules/auth/account-form";

export default function ForgotPasswordPage() {
  return (
    <main>
      <AccountForm action="auth/forgot" />
    </main>
  );
}
