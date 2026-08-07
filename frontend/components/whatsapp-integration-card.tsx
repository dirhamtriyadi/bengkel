"use client";

import { zodResolver } from "@hookform/resolvers/zod";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { CheckCircle2, Loader2, MessageCircle, Play, QrCode, RefreshCw, Save } from "lucide-react";
import Image from "next/image";
import { useEffect, useState } from "react";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { apiClient } from "@/lib/api";

type Integration = {
  enabled: boolean;
  base_url: string;
  session_id: string;
  api_token_configured: boolean;
};
type SessionStatus = { connected: boolean; state: string; message: string; session_id: string };
type QRResponse = { image: string; session_id: string };

const schema = z.object({
  enabled: z.boolean(),
  base_url: z.string().trim().max(500),
  api_token: z.string().max(2048),
  session_id: z.string().trim().max(30),
}).superRefine((value, context) => {
  if (!value.enabled) return;
  if (!z.string().url().safeParse(value.base_url).success || !/^https?:\/\//i.test(value.base_url)) {
    context.addIssue({ code: z.ZodIssueCode.custom, path: ["base_url"], message: "Gunakan URL http(s) lengkap" });
  }
  if (!value.session_id) {
    context.addIssue({ code: z.ZodIssueCode.custom, path: ["session_id"], message: "Nomor session wajib diisi" });
  }
});
type Form = z.infer<typeof schema>;

export function WhatsAppIntegrationCard() {
  const queryClient = useQueryClient();
  const [notice, setNotice] = useState("");
  const [actionError, setActionError] = useState("");
  const [starting, setStarting] = useState(false);
  const configuration = useQuery({ queryKey: ["whatsapp-integration"], queryFn: () => apiClient<Integration>("/integrations/whatsapp") });
  const integration = configuration.data?.data;
  const { register, handleSubmit, reset, setError, formState: { errors, isSubmitting } } = useForm<Form>({
    resolver: zodResolver(schema),
    defaultValues: { enabled: false, base_url: "", api_token: "", session_id: "" },
  });

  useEffect(() => {
    if (integration) reset({ enabled: integration.enabled, base_url: integration.base_url, api_token: "", session_id: integration.session_id });
  }, [integration, reset]);

  const canManageSession = Boolean(integration?.enabled && integration.api_token_configured && integration.base_url && integration.session_id);
  const status = useQuery({
    queryKey: ["whatsapp-session-status"],
    queryFn: () => apiClient<SessionStatus>("/integrations/whatsapp/session/status"),
    enabled: canManageSession,
    retry: false,
    refetchInterval: (result) => result.state.data?.data?.connected ? 15000 : 5000,
  });
  const qr = useQuery({
    queryKey: ["whatsapp-session-qr"],
    queryFn: () => apiClient<QRResponse>("/integrations/whatsapp/session/qr"),
    enabled: false,
    retry: false,
  });

  async function save(values: Form) {
    setNotice("");
    setActionError("");
    if (values.enabled && !integration?.api_token_configured && !values.api_token.trim()) {
      setError("api_token", { message: "Token API wajib diisi saat pertama kali mengaktifkan integrasi" });
      return;
    }
    try {
      await apiClient<Integration>("/integrations/whatsapp", { method: "PUT", body: JSON.stringify(values) });
      await queryClient.invalidateQueries({ queryKey: ["whatsapp-integration"] });
      await queryClient.invalidateQueries({ queryKey: ["settings"] });
      setNotice("Konfigurasi WhatsApp tersimpan aman di database.");
    } catch (caught) {
      setActionError(caught instanceof Error ? caught.message : "Konfigurasi WhatsApp gagal disimpan");
    }
  }

  async function startSession() {
    setStarting(true);
    setNotice("");
    setActionError("");
    try {
      await apiClient("/integrations/whatsapp/session/start", { method: "POST" });
      setNotice("Session dimulai. Pindai QR dengan nomor WhatsApp pengirim.");
      await status.refetch();
      await qr.refetch();
    } catch (caught) {
      setActionError(caught instanceof Error ? caught.message : "Session WhatsApp gagal dimulai");
    } finally {
      setStarting(false);
    }
  }

  return <Card>
    <CardHeader>
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div><CardTitle className="flex items-center gap-2"><MessageCircle className="size-5"/>WhatsApp invoice (wwebjs-api)</CardTitle><CardDescription className="mt-2">Hubungkan BengkelOS ke deployment wwebjs-api yang terpisah. Token API dienkripsi di database dan tidak pernah ditampilkan kembali.</CardDescription></div>
        {canManageSession && <Badge className={status.data?.data?.connected ? "bg-emerald-100 text-emerald-700" : "bg-orange-100 text-orange-700"}>{status.data?.data?.connected ? "Terhubung" : status.isFetching ? "Memeriksa..." : "Belum terhubung"}</Badge>}
      </div>
    </CardHeader>
    <CardContent>
      <form className="grid gap-4 lg:grid-cols-2" onSubmit={handleSubmit(save)}>
        <label className="lg:col-span-2 flex items-start gap-3 rounded-xl border p-4 text-sm"><input type="checkbox" className="mt-1 size-4" {...register("enabled")}/><span><strong className="block">Aktifkan pengiriman invoice WhatsApp</strong><span className="text-muted-foreground">BengkelOS akan memanggil URL service terpisah menggunakan header x-api-key.</span></span></label>
        <label className="text-sm font-medium">URL wwebjs-api<Input className="mt-2" placeholder="https://wwebjs.example.com" {...register("base_url")}/>{errors.base_url&&<span className="mt-1 block text-xs text-destructive">{errors.base_url.message}</span>}</label>
        <label className="text-sm font-medium">Nomor session / pengirim<Input className="mt-2" placeholder="6281234567890" {...register("session_id")}/>{errors.session_id&&<span className="mt-1 block text-xs text-destructive">{errors.session_id.message}</span>}<span className="mt-1 block text-xs text-muted-foreground">Nomor lokal 08... otomatis disimpan sebagai 628...</span></label>
        <label className="text-sm font-medium lg:col-span-2">Token API<Input className="mt-2" type="password" autoComplete="new-password" placeholder={integration?.api_token_configured ? "Token sudah tersimpan — kosongkan untuk mempertahankan" : "Masukkan API key wwebjs-api"} {...register("api_token")}/>{integration?.api_token_configured&&<span className="mt-1 flex items-center gap-1 text-xs text-emerald-700"><CheckCircle2 className="size-3"/>Token terenkripsi sudah tersimpan</span>}{errors.api_token&&<span className="mt-1 block text-xs text-destructive">{errors.api_token.message}</span>}</label>
        <div className="lg:col-span-2"><Button disabled={isSubmitting}>{isSubmitting?<Loader2 className="size-4 animate-spin"/>:<Save className="size-4"/>}{isSubmitting?"Menyimpan...":"Simpan konfigurasi WhatsApp"}</Button></div>
      </form>

      {(notice||actionError)&&<p className={`mt-4 rounded-xl p-3 text-sm ${actionError?"bg-red-50 text-red-700":"bg-emerald-50 text-emerald-700"}`}>{actionError||notice}</p>}

      {canManageSession&&<div className="mt-6 border-t pt-6"><div className="flex flex-wrap items-center justify-between gap-3"><div><p className="font-bold">Pairing session {integration?.session_id}</p><p className="mt-1 text-sm text-muted-foreground">Status: {status.data?.data?.state||status.data?.data?.message||status.error?.message||"belum dimulai"}</p></div><div className="flex flex-wrap gap-2"><Button type="button" variant="outline" disabled={status.isFetching} onClick={()=>status.refetch()}><RefreshCw className={`size-4 ${status.isFetching?"animate-spin":""}`}/>Cek status</Button>{!status.data?.data?.connected&&<><Button type="button" disabled={starting} onClick={startSession}>{starting?<Loader2 className="size-4 animate-spin"/>:<Play className="size-4"/>}{starting?"Memulai...":"Mulai session"}</Button><Button type="button" variant="outline" disabled={qr.isFetching} onClick={()=>qr.refetch()}><QrCode className="size-4"/>{qr.isFetching?"Mengambil...":"Muat QR"}</Button></>}</div></div>{qr.error&&<p className="mt-4 rounded-xl bg-orange-50 p-3 text-sm text-orange-700">{qr.error.message}</p>}{qr.data?.data?.image&&!status.data?.data?.connected&&<div className="mt-5 grid gap-5 rounded-2xl border bg-white p-5 sm:grid-cols-[280px_1fr]"><Image src={qr.data.data.image} alt={`QR pairing WhatsApp ${qr.data.data.session_id}`} width={280} height={280} unoptimized className="rounded-xl border"/><div className="self-center text-sm"><p className="font-bold">Pindai dari WhatsApp pengirim</p><ol className="mt-3 list-decimal space-y-2 pl-5 text-muted-foreground"><li>Buka WhatsApp pada nomor {integration?.session_id}.</li><li>Pilih menu Perangkat tertaut.</li><li>Pilih Tautkan perangkat lalu pindai QR.</li><li>Kembali ke sini dan tekan Cek status.</li></ol></div></div>}</div>}
    </CardContent>
  </Card>;
}
