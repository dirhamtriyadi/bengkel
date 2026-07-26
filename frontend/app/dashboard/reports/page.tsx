"use client";

import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { Landmark,PackageSearch,Scale,TrendingDown,TrendingUp,Wallet } from "lucide-react";
import { DataTable } from "@/components/data-table";
import { Button } from "@/components/ui/button";
import { Card,CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { apiClient } from "@/lib/api";
import { dateTime,rupiah } from "@/lib/utils";

type AccountRow={id?:string;code:string;name:string;type:string;debit:number;credit:number;balance:number};
type ProfitLoss={from:string;to:string;total_revenue:number;total_expense:number;net_profit:number;accounts:AccountRow[]};
type Trial={to:string;accounts:AccountRow[];total_debit:number;total_credit:number;balanced:boolean};
type Balance={to:string;accounts:AccountRow[];total_assets:number;total_liabilities:number;total_equity:number;retained_earnings:number;total_liabilities_equity:number};
type Cash={from:string;to:string;transactions:Array<{date:string;journal_number:string;description:string;inflow:number;outflow:number}>;total_inflow:number;total_outflow:number;net_cash_flow:number};
type Sales={rows:Array<{date:string;method:string;transactions:number;gross_sales:number;gateway_fee:number}>};
type Inventory={rows:Array<{product_id:string;sku:string;name:string;quantity:number;unit_cost:number;value:number}>;total_value:number};
type Tab="profit-loss"|"trial-balance"|"balance-sheet"|"cash-flow"|"general-ledger"|"sales"|"inventory";

export default function Reports(){
 const now=new Date();const [from,setFrom]=useState(new Date(now.getFullYear(),now.getMonth(),1).toISOString().slice(0,10));const [to,setTo]=useState(now.toISOString().slice(0,10));const [tab,setTab]=useState<Tab>("profit-loss");const period=`from=${from}&to=${to}`;
 const profit=useQuery({queryKey:["report","profit",period],queryFn:()=>apiClient<ProfitLoss>(`/reports/profit-loss?${period}`),enabled:tab==="profit-loss"});
 const trial=useQuery({queryKey:["report","trial",to],queryFn:()=>apiClient<Trial>(`/reports/trial-balance?to=${to}`),enabled:tab==="trial-balance"});
 const balance=useQuery({queryKey:["report","balance",to],queryFn:()=>apiClient<Balance>(`/reports/balance-sheet?to=${to}`),enabled:tab==="balance-sheet"});
 const cash=useQuery({queryKey:["report","cash",period],queryFn:()=>apiClient<Cash>(`/reports/cash-flow?${period}`),enabled:tab==="cash-flow"});
 const sales=useQuery({queryKey:["report","sales",period],queryFn:()=>apiClient<Sales>(`/reports/sales?${period}`),enabled:tab==="sales"});
 const inventory=useQuery({queryKey:["report","inventory"],queryFn:()=>apiClient<Inventory>("/reports/inventory-valuation"),enabled:tab==="inventory"});
 const tabs:Array<[Tab,string]>=[["profit-loss","Laba rugi"],["trial-balance","Neraca saldo"],["balance-sheet","Neraca"],["cash-flow","Arus kas"],["general-ledger","Buku besar"],["sales","Penjualan"],["inventory","Nilai persediaan"]];
 const pl=profit.data?.data;const tb=trial.data?.data;const bs=balance.data?.data;const cf=cash.data?.data;const sr=sales.data?.data;const iv=inventory.data?.data;
 return <div className="space-y-6"><div><h1 className="text-3xl font-bold tracking-tight">Akuntansi & laporan</h1><p className="mt-2 text-muted-foreground">Laporan berbasis jurnal terposting dengan drill-down operasional.</p></div><div className="flex flex-col gap-3 rounded-2xl border bg-white p-4 lg:flex-row lg:items-end lg:justify-between"><div className="flex flex-wrap gap-2">{tabs.map(([value,label])=><Button key={value} variant={tab===value?"default":"outline"} size="sm" onClick={()=>setTab(value)}>{label}</Button>)}</div><div className="flex gap-2"><label className="text-xs text-muted-foreground">Dari<Input type="date" className="mt-1" value={from} onChange={event=>setFrom(event.target.value)}/></label><label className="text-xs text-muted-foreground">Sampai<Input type="date" className="mt-1" value={to} onChange={event=>setTo(event.target.value)}/></label></div></div>
 {tab==="profit-loss"&&<><Stats values={[[TrendingUp,"Pendapatan",pl?.total_revenue??0,"text-emerald-600"],[TrendingDown,"Beban",pl?.total_expense??0,"text-red-600"],[Wallet,"Laba bersih",pl?.net_profit??0,"text-blue-600"]]}/><AccountTable rows={pl?.accounts??[]} title="Rincian laba rugi"/></>}
 {tab==="trial-balance"&&<><Stats values={[[TrendingUp,"Total debit",tb?.total_debit??0,"text-blue-600"],[TrendingDown,"Total kredit",tb?.total_credit??0,"text-violet-600"],[Scale,tb?.balanced?"Seimbang":"Tidak seimbang",Math.abs((tb?.total_debit??0)-(tb?.total_credit??0)),tb?.balanced?"text-emerald-600":"text-red-600"]]}/><AccountTable rows={tb?.accounts??[]} title="Neraca saldo"/></>}
 {tab==="balance-sheet"&&<><Stats values={[[Landmark,"Total aset",bs?.total_assets??0,"text-blue-600"],[TrendingDown,"Liabilitas",bs?.total_liabilities??0,"text-red-600"],[Scale,"Ekuitas + laba ditahan",((bs?.total_equity??0)+(bs?.retained_earnings??0)),"text-emerald-600"]]}/><AccountTable rows={bs?.accounts??[]} title="Posisi keuangan"/></>}
 {tab==="cash-flow"&&<><Stats values={[[TrendingUp,"Kas masuk",cf?.total_inflow??0,"text-emerald-600"],[TrendingDown,"Kas keluar",cf?.total_outflow??0,"text-red-600"],[Wallet,"Arus kas bersih",cf?.net_cash_flow??0,"text-blue-600"]]}/><SimpleTable headers={["Tanggal","Jurnal","Keterangan","Masuk","Keluar"]} rows={(cf?.transactions??[]).map(row=>[dateTime.format(new Date(row.date)),row.journal_number,row.description,rupiah.format(row.inflow),rupiah.format(row.outflow)])}/></>}
 {tab==="general-ledger"&&<DataTable endpoint="/reports/general-ledger" columns={[{key:"entry_date",label:"Tanggal",format:value=>dateTime.format(new Date(String(value)))},{key:"journal_number",label:"Jurnal"},{key:"account_code",label:"Kode akun"},{key:"account_name",label:"Akun"},{key:"description",label:"Keterangan"},{key:"debit",label:"Debit",format:value=>rupiah.format(Number(value))},{key:"credit",label:"Kredit",format:value=>rupiah.format(Number(value))}]}/>}
 {tab==="sales"&&<SimpleTable headers={["Tanggal","Metode","Transaksi","Penjualan bruto","Biaya gateway"]} rows={(sr?.rows??[]).map(row=>[dateTime.format(new Date(row.date)),row.method,String(row.transactions),rupiah.format(row.gross_sales),rupiah.format(row.gateway_fee)])}/>}
 {tab==="inventory"&&<><Stats values={[[PackageSearch,"Nilai persediaan",iv?.total_value??0,"text-blue-600"]]}/><SimpleTable headers={["SKU","Produk","Kuantitas","Harga modal","Nilai"]} rows={(iv?.rows??[]).map(row=>[row.sku,row.name,String(row.quantity),rupiah.format(row.unit_cost),rupiah.format(row.value)])}/></>}
 </div>
}

function Stats({values}:{values:Array<[React.ElementType,string,number,string]>}){return <div className={`grid gap-4 ${values.length>1?"md:grid-cols-3":"md:grid-cols-1"}`}>{values.map(([Icon,label,value,tone])=><Card key={label}><CardContent className="flex items-center justify-between p-6"><div><p className="text-sm text-muted-foreground">{label}</p><p className="mt-2 text-2xl font-bold">{rupiah.format(value)}</p></div><Icon className={`size-7 ${tone}`}/></CardContent></Card>)}</div>}
function AccountTable({rows,title}:{rows:AccountRow[];title:string}){return <div className="rounded-2xl border bg-white p-5"><h2 className="mb-4 text-lg font-bold">{title}</h2><SimpleTable headers={["Kode","Akun","Tipe","Debit","Kredit","Saldo"]} rows={rows.map(row=>[row.code,row.name,row.type,rupiah.format(row.debit),rupiah.format(row.credit),rupiah.format(row.balance)])}/></div>}
function SimpleTable({headers,rows}:{headers:string[];rows:string[][]}){return <div className="overflow-x-auto rounded-2xl border bg-white"><table className="w-full text-sm"><thead className="bg-muted/60"><tr>{headers.map(header=><th key={header} className="whitespace-nowrap p-3 text-left">{header}</th>)}</tr></thead><tbody>{rows.map((row,index)=><tr key={index} className="border-t">{row.map((cell,cellIndex)=><td key={cellIndex} className="whitespace-nowrap p-3">{cell}</td>)}</tr>)}{rows.length===0&&<tr><td colSpan={headers.length} className="p-10 text-center text-muted-foreground">Belum ada data pada periode ini.</td></tr>}</tbody></table></div>}
