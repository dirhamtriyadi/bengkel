"use client";
import { Copy,ExternalLink,Eye,Loader2,MessageCircle,Send } from "lucide-react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { ChangePaymentMethodDialog, type ChangeablePayment } from "@/components/change-payment-method-dialog";
import { DataTable,type TableColumn,type TableRow } from "@/components/data-table";
import { Dialog,DialogContent,DialogDescription,DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { apiClient } from "@/lib/api";
import { dateTime,rupiah } from "@/lib/utils";

const status=(value:unknown)=><Badge className={String(value)==="paid"||String(value)==="completed"?"bg-emerald-100 text-emerald-700":String(value)==="cancelled"||String(value)==="void"?"bg-red-100 text-red-700":"bg-orange-100 text-orange-700"}>{String(value)}</Badge>;
const money=(value:unknown)=>rupiah.format(Number(value??0));
const date=(value:unknown)=>value?dateTime.format(new Date(String(value))):"—";
const configs:Record<string,{title:string;description:string;endpoint:string;columns:TableColumn[]}> = {
  customers:{title:"Pelanggan",description:"Kelola profil dan riwayat pelanggan lintas transaksi.",endpoint:"/customers",columns:[{key:"code",label:"Kode"},{key:"name",label:"Nama"},{key:"phone",label:"Telepon"},{key:"email",label:"Email"},{key:"created_at",label:"Terdaftar",format:date}]},
  vehicles:{title:"Kendaraan",description:"Identitas motor dapat berupa plat nomor atau kode internal.",endpoint:"/vehicles",columns:[{key:"identifier",label:"Identitas"},{key:"plate_number",label:"Plat nomor"},{key:"brand",label:"Merek"},{key:"model",label:"Model"},{key:"year",label:"Tahun"},{key:"odometer",label:"Odometer"}]},
  products:{title:"Produk & jasa",description:"Satu katalog konsisten untuk suku cadang, jasa, dan item lain.",endpoint:"/products",columns:[{key:"sku",label:"SKU"},{key:"name",label:"Nama"},{key:"type",label:"Tipe",format:status},{key:"unit",label:"Satuan"},{key:"cost_price",label:"Harga modal",format:money},{key:"selling_price",label:"Harga jual",format:money}]},
  inventory:{title:"Persediaan",description:"Saldo stok per cabang dan peringatan minimum.",endpoint:"/inventory",columns:[{key:"sku",label:"SKU"},{key:"name",label:"Produk"},{key:"quantity",label:"Stok"},{key:"unit",label:"Satuan"},{key:"min_stock",label:"Minimum"},{key:"updated_at",label:"Diperbarui",format:date}]},
  "work-orders":{title:"Work order",description:"Pantau kendaraan dari inspeksi sampai menjadi invoice.",endpoint:"/work-orders",columns:[{key:"number",label:"Nomor"},{key:"status",label:"Status",format:status},{key:"complaint",label:"Keluhan"},{key:"odometer",label:"Odometer"},{key:"created_at",label:"Dibuat",format:date}]},
  sales:{title:"Transaksi",description:"Penjualan servis dan retail dalam satu arus kas.",endpoint:"/sales",columns:[{key:"number",label:"Invoice"},{key:"customer_name",label:"Pelanggan"},{key:"customer_phone",label:"WhatsApp"},{key:"status",label:"Status",format:status},{key:"subtotal",label:"Subtotal",format:money},{key:"gateway_fee",label:"Biaya gateway",format:money},{key:"grand_total",label:"Total",format:money},{key:"paid_at",label:"Dibayar",format:date}]},
  payments:{title:"Pembayaran",description:"Pantau pembayaran cash dan Midtrans beserta channel, fee pelanggan, dan beban provider.",endpoint:"/payments",columns:[{key:"sale_number",label:"Invoice"},{key:"method",label:"Metode",format:status},{key:"payment_channel",label:"Channel",format:status},{key:"status",label:"Status",format:status},{key:"base_amount",label:"Tagihan",format:money},{key:"customer_fee",label:"Fee pelanggan",format:money},{key:"amount",label:"Dibayar",format:money},{key:"provider_fee",label:"Fee provider",format:money},{key:"fee_bearer",label:"Penanggung fee"},{key:"provider_reference",label:"Referensi provider"},{key:"paid_at",label:"Dibayar pada",format:date}]},
  "audit-logs":{title:"Audit log",description:"Jejak aksi sensitif yang immutable dan dapat ditelusuri.",endpoint:"/audit-logs",columns:[{key:"created_at",label:"Waktu",format:date},{key:"action",label:"Aksi",format:status},{key:"resource",label:"Resource"},{key:"resource_id",label:"Resource ID"},{key:"request_id",label:"Request ID"},{key:"ip_address",label:"IP"}]}
};

type SendInvoiceResult={invoice_number:string;public_url:string;expires_at:string;whatsapp:{sent:boolean;recipient:string;message_id?:string}};

function SendInvoiceAction({row}:{row:TableRow}){
  const eligible=String(row.status)==="pending"&&String(row.payment_method)==="midtrans";
  const hasPhone=Boolean(String(row.customer_phone??"").trim());
  const [open,setOpen]=useState(false);const [sending,setSending]=useState(false);const [error,setError]=useState("");const [copied,setCopied]=useState(false);const [result,setResult]=useState<SendInvoiceResult>();
  if(!eligible)return <span className="text-muted-foreground">—</span>;
  async function sendInvoice(){setSending(true);setError("");setCopied(false);try{const response=await apiClient<SendInvoiceResult>(`/sales/${String(row.id)}/public-invoice/whatsapp`,{method:"POST"});if(response.data)setResult(response.data)}catch(caught){setError(caught instanceof Error?caught.message:"Invoice gagal dikirim")}finally{setSending(false)}}
  async function copyLink(){if(!result)return;try{await navigator.clipboard.writeText(result.public_url);setCopied(true)}catch{setError("Tautan tidak dapat disalin otomatis. Salin dari kolom di bawah.")}}
  return <Dialog open={open} onOpenChange={value=>{setOpen(value);if(!value){setError("");setCopied(false);setResult(undefined)}}}><Button size="sm" variant="outline" disabled={!hasPhone} title={hasPhone?"Kirim invoice publik":"Pelanggan belum memiliki nomor WhatsApp"} onClick={()=>setOpen(true)}><MessageCircle className="size-4"/>Kirim invoice</Button><DialogContent className="max-w-lg"><DialogTitle>Kirim invoice lewat WhatsApp</DialogTitle><DialogDescription>Invoice {String(row.number)} akan dikirim ke nomor pada profil pelanggan. Tautan dapat dibuka dan dibayar tanpa login.</DialogDescription>{!result?<div className="mt-6"><div className="rounded-xl bg-muted p-4 text-sm"><p className="font-semibold">{String(row.customer_name||"Pelanggan")}</p><p className="mt-1 text-muted-foreground">{String(row.customer_phone)}</p><p className="mt-3 font-bold">{rupiah.format(Number(row.grand_total??0))}</p></div>{error&&<p className="mt-4 rounded-xl bg-red-50 p-3 text-sm text-red-700">{error}</p>}<div className="mt-5 flex justify-end gap-2"><Button variant="outline" onClick={()=>setOpen(false)}>Batal</Button><Button disabled={sending} onClick={sendInvoice}>{sending?<Loader2 className="size-4 animate-spin"/>:<Send className="size-4"/>}{sending?"Mengirim...":"Kirim sekarang"}</Button></div></div>:<div className="mt-6"><div className="rounded-xl border border-emerald-200 bg-emerald-50 p-4 text-sm text-emerald-800"><p className="font-bold">Invoice berhasil dikirim</p><p className="mt-1">Tujuan {result.whatsapp.recipient} · berlaku sampai {dateTime.format(new Date(result.expires_at))}</p></div><label className="mt-4 block text-sm font-semibold">Tautan publik<Input readOnly className="mt-2 font-mono text-xs" value={result.public_url}/></label>{error&&<p className="mt-3 text-sm text-red-700">{error}</p>}<div className="mt-5 flex flex-wrap justify-end gap-2"><Button variant="outline" onClick={copyLink}><Copy className="size-4"/>{copied?"Tersalin":"Salin"}</Button><Button variant="outline" onClick={()=>window.open(result.public_url,"_blank","noopener,noreferrer")}><ExternalLink className="size-4"/>Buka invoice</Button><Button onClick={()=>setOpen(false)}>Selesai</Button></div></div>}</DialogContent></Dialog>
}

function SalesActions({row}:{row:TableRow}){
  return <div className="flex flex-wrap gap-2">
    <Link className="inline-flex h-9 items-center justify-center gap-2 rounded-lg border bg-background px-3 text-sm font-semibold hover:bg-muted" href={`/print/receipt/${String(row.id)}`}><Eye className="size-4"/>Lihat</Link>
    <SendInvoiceAction row={row}/>
  </div>;
}

function PaymentActions({row}:{row:TableRow}){
  const metadata=(row.metadata??{}) as Record<string,unknown>;
  const eligible=String(row.sale_status)==="pending"&&["pending","failed","expired"].includes(String(row.status))&&!metadata.superseded_by;
  const payment:ChangeablePayment={id:String(row.id),sale_id:String(row.sale_id),sale_number:String(row.sale_number??""),method:String(row.method),status:String(row.status),base_amount:Number(row.base_amount??0),amount:Number(row.amount??0)};
  return <div className="flex flex-wrap gap-2">
    <Link className="inline-flex h-9 items-center justify-center gap-2 rounded-lg border bg-background px-3 text-sm font-semibold hover:bg-muted" href={`/print/receipt/${payment.sale_id}`}><Eye className="size-4"/>Invoice</Link>
    {eligible&&<ChangePaymentMethodDialog payment={payment}/>}
  </div>;
}

export default function ResourcePage(){const {resource}=useParams<{resource:string}>();const config=configs[resource];if(!config)return <div className="rounded-2xl border bg-white p-10">Halaman tidak ditemukan.</div>;const actions=resource==="sales"?(row:TableRow)=><SalesActions row={row}/>:resource==="payments"?(row:TableRow)=><PaymentActions row={row}/>:undefined;return <div className="space-y-6"><div><h1 className="text-3xl font-bold tracking-tight">{config.title}</h1><p className="mt-2 text-muted-foreground">{config.description}</p></div><DataTable endpoint={config.endpoint} columns={config.columns} actions={actions}/></div>}
