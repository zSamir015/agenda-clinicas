import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import {
  Alert,
  Badge,
  Button,
  Card,
  CardDescription,
  CardHeader,
  CardTitle,
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogTitle,
  DialogTrigger,
  Field,
  Input,
  ThemeToggle,
} from "./index";

describe("UI base components", () => {
  it("renders Button with loading state and disables interaction", () => {
    render(
      <Button variant="primary" loading>
        Guardar
      </Button>,
    );
    const btn = screen.getByRole("button", { name: /guardar/i });
    expect(btn).toBeDisabled();
    expect(btn).toHaveAttribute("aria-busy", "true");
  });

  it("wires Field label, hint, and error to Input for accessibility", () => {
    render(
      <Field
        label="Cédula"
        hint="Formato 8-0000-00001"
        error="Cédula obligatoria"
        required
      >
        <Input placeholder="8-0000-00001" />
      </Field>,
    );

    const input = screen.getByLabelText(/cédula/i);
    expect(input).toHaveAttribute("aria-invalid", "true");
    expect(input).toBeRequired();

    const alert = screen.getByRole("alert");
    expect(alert).toHaveTextContent("Cédula obligatoria");
    const describedBy = input.getAttribute("aria-describedby") ?? "";
    expect(describedBy).toContain(alert.id);
  });

  it("renders Card, Badge, and Alert with semantic roles", () => {
    render(
      <Card>
        <CardHeader>
          <CardTitle>Estado del sistema</CardTitle>
          <CardDescription>Resumen operativo</CardDescription>
        </CardHeader>
        <Badge tone="success">Confirmada</Badge>
        <Alert tone="danger" title="Error">
          No se pudo conectar
        </Alert>
      </Card>,
    );

    expect(
      screen.getByRole("heading", { name: "Estado del sistema" }),
    ).toBeInTheDocument();
    expect(screen.getByText("Confirmada")).toBeInTheDocument();
    expect(screen.getByRole("alert")).toHaveTextContent("No se pudo conectar");
  });

  it("opens and closes Radix Dialog", () => {
    render(
      <Dialog>
        <DialogTrigger asChild>
          <Button>Abrir</Button>
        </DialogTrigger>
        <DialogContent>
          <DialogTitle>Confirmar cita</DialogTitle>
          <DialogDescription>Revise la hora antes de seguir.</DialogDescription>
          <DialogClose asChild>
            <Button variant="secondary">Cerrar</Button>
          </DialogClose>
        </DialogContent>
      </Dialog>,
    );

    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Abrir" }));
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(screen.getByText("Confirmar cita")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Cerrar" }));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("toggles data-theme and persists choice in localStorage", () => {
    window.localStorage.removeItem("agenda-theme");
    document.documentElement.removeAttribute("data-theme");

    render(<ThemeToggle />);
    const toggle = screen.getByRole("button");
    expect(toggle).toHaveAttribute("aria-pressed", "false");

    fireEvent.click(toggle);
    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
    expect(window.localStorage.getItem("agenda-theme")).toBe("dark");
    expect(toggle).toHaveAttribute("aria-pressed", "true");
  });
});
