import { Suspense } from "react";
import { InvitationFlow } from "@/components/auth/invitation-flow";

export default function InvitacionPage() {
  return (
    <Suspense
      fallback={
        <main className="mx-auto flex min-h-dvh max-w-xl items-center justify-center p-4">
          <p className="text-[length:var(--text-sm)] text-[var(--text-secondary)]">
            Cargando invitación...
          </p>
        </main>
      }
    >
      <InvitationFlow />
    </Suspense>
  );
}
