"use client";

import * as React from "react";
import { Copy, Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import {
  createAdminRegistrationCodes,
  deleteAdminRegistrationCode,
  listAdminRegistrationCodes,
  type AdminRegistrationCodeDTO,
} from "@/features/admin/api";
import { resolveAccessToken } from "@/shared/auth/resolve-access-token";
import { writeClipboardText } from "@/shared/lib/clipboard";

export function AdminRegistrationCodesPage() {
  const [items, setItems] = React.useState<AdminRegistrationCodeDTO[]>([]);
  const [quantity, setQuantity] = React.useState("10");
  const [loading, setLoading] = React.useState(true);
  const [creating, setCreating] = React.useState(false);

  const copyCode = async (value: string) => {
    try {
      await writeClipboardText(value);
      toast.success("Copied registration code.");
    } catch {
      toast.error("Failed to copy registration code.");
    }
  };

  const load = React.useCallback(async () => {
    const token = await resolveAccessToken();
    if (!token) return;
    setLoading(true);
    try {
      const result = await listAdminRegistrationCodes(token);
      setItems(result.results ?? []);
    } finally {
      setLoading(false);
    }
  }, []);

  React.useEffect(() => {
    void load();
  }, [load]);

  const create = async () => {
    const token = await resolveAccessToken();
    if (!token) return;
    const count = Math.max(1, Math.min(100, Number(quantity) || 1));
    setCreating(true);
    try {
      const result = await createAdminRegistrationCodes(token, count);
      setItems((current) => [...result.results, ...current]);
      await copyCode(result.results.map((item) => item.code).join("\n"));
      toast.success(`Generated ${result.results.length} registration codes.`);
    } catch {
      toast.error("Failed to generate registration codes.");
    } finally {
      setCreating(false);
    }
  };

  const remove = async (id: number) => {
    const token = await resolveAccessToken();
    if (!token) return;
    try {
      await deleteAdminRegistrationCode(token, id);
      await load();
    } catch {
      toast.error("Failed to delete registration code.");
    }
  };

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-2xl font-semibold">Registration codes</h2>
        <p className="mt-1 text-sm text-muted-foreground">Each code registers one account. Plaintext is retained for management.</p>
      </div>
      <div className="flex max-w-md items-center gap-2">
        <Input type="number" min={1} max={100} value={quantity} onChange={(event) => setQuantity(event.target.value)} />
        <Button onClick={() => void create()} disabled={creating}>
          <Plus className="mr-2 size-4" />
          Generate codes
        </Button>
      </div>
      <div className="rounded-md border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Code</TableHead>
              <TableHead>Status</TableHead>
              <TableHead>Registered user ID</TableHead>
              <TableHead className="text-right">Actions</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {loading ? (
              <TableRow><TableCell colSpan={4}>Loading...</TableCell></TableRow>
            ) : items.length === 0 ? (
              <TableRow><TableCell colSpan={4}>No registration codes.</TableCell></TableRow>
            ) : items.map((item) => (
              <TableRow key={item.id}>
                <TableCell className="font-mono">{item.code}</TableCell>
                <TableCell>{item.status}</TableCell>
                <TableCell>{item.usedByUserID || "-"}</TableCell>
                <TableCell className="text-right">
                  <div className="flex justify-end gap-1">
                    <Button size="icon" variant="ghost" title="Copy" onClick={() => void copyCode(item.code)}><Copy className="size-4" /></Button>
                    {item.status === "active" ? <Button size="icon" variant="ghost" title="Delete" onClick={() => void remove(item.id)}><Trash2 className="size-4" /></Button> : null}
                  </div>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
    </div>
  );
}
