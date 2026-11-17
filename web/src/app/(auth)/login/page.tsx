import { Suspense } from "react";
import { LoginForm } from "@/components/auth/login-form";

export default function LoginPage() {
  return (
    <Suspense
      fallback={
        <main className="mx-auto flex min-h-dvh max-w-md items-center justify-center p-4">
          <p className="text-[var(--text-sm)] text-[var(--text-secondary)]">
            Cargando inicio de sesión...
          </p>
        </main>
      }
    >
      <LoginForm />
    </Suspense>
  );
}
