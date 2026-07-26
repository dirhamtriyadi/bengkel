"use client";

import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation,useQueryClient } from "@tanstack/react-query";
import { Pencil,Plus,PowerOff } from "lucide-react";
import { useState } from "react";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { DataTable,type TableRow } from "@/components/data-table";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Dialog,DialogContent,DialogDescription,DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { apiClient } from "@/lib/api";
import { rupiah } from "@/lib/utils";

const schema=z.object({sku:z.string().min(1),name:z.string().min(1),type:z.enum(["part","service","other"]),unit:z.string().min(1),cost_price:z.coerce.number().min(0),selling_price:z.coerce.number().min(0),min_stock:z.coerce.number().min(0),is_active:z.boolean()});
type Form=z.infer<typeof schema>;const defaults:Form={sku:"",name:"",type:"part",unit:"pcs",cost_price:0,selling_price:0,min_stock:0,is_active:true};

export default function ProductsPage(){
 const client=useQueryClient();const [open,setOpen]=useState(false);const [editing,setEditing]=useState<TableRow|null>(null);const {register,handleSubmit,reset,formState:{errors}}=useForm<Form>({resolver:zodResolver(schema),defaultValues:defaults});
 const mutation=useMutation({mutationFn:(values:Form)=>apiClient(editing?`/products/${editing.id}`:"/products",{method:editing?"PUT":"POST",body:JSON.stringify(values)}),onSuccess:()=>{client.invalidateQueries({queryKey:["/products"]});client.invalidateQueries({queryKey:["pos-products"]});client.invalidateQueries({queryKey:["work-order-products"]});setOpen(false)}});
 function show(row?:TableRow){setEditing(row??null);reset(row?{sku:String(row.sku),name:String(row.name),type:row.type as Form["type"],unit:String(row.unit),cost_price:Number(row.cost_price),selling_price:Number(row.selling_price),min_stock:Number(row.min_stock),is_active:Boolean(row.is_active)}:defaults);setOpen(true)}
 async function deactivate(row:TableRow){if(!window.confirm(`Nonaktifkan ${row.name}?`))return;await apiClient(`/products/${row.id}`,{method:"DELETE"});client.invalidateQueries({queryKey:["/products"]})}
 return <div className="space-y-6"><div><h1 className="text-3xl font-bold tracking-tight">Produk & jasa</h1><p className="mt-2 text-muted-foreground">Katalog barang yang memakai stok dan jasa yang dapat dimasukkan montir.</p></div><DataTable endpoint="/products" defaultSort="name" columns={[{key:"sku",label:"SKU"},{key:"name",label:"Nama"},{key:"type",label:"Tipe",format:value=><Badge>{String(value)}</Badge>},{key:"unit",label:"Satuan"},{key:"cost_price",label:"Harga modal",format:value=>rupiah.format(Number(value))},{key:"selling_price",label:"Harga jual",format:value=>rupiah.format(Number(value))},{key:"min_stock",label:"Stok minimum"},{key:"is_active",label:"Status",format:value=>value?"Aktif":"Nonaktif"}]} toolbar={<Button onClick={()=>show()}><Plus className="size-4"/>Tambah produk</Button>} actions={row=><div className="flex gap-1"><Button variant="ghost" size="icon" onClick={()=>show(row)}><Pencil className="size-4"/></Button>{Boolean(row.is_active)&&<Button variant="ghost" size="icon" onClick={()=>deactivate(row)}><PowerOff className="size-4 text-red-600"/></Button>}</div>}/><Dialog open={open} onOpenChange={setOpen}><DialogContent><DialogTitle>{editing?"Edit produk":"Tambah produk atau jasa"}</DialogTitle><DialogDescription>Barang bertipe part memengaruhi persediaan; service tidak memakai stok.</DialogDescription><form className="mt-6 grid gap-4 sm:grid-cols-2" onSubmit={handleSubmit(values=>mutation.mutate(values))}><Field label="SKU" error={errors.sku?.message}><Input {...register("sku")}/></Field><Field label="Nama" error={errors.name?.message}><Input {...register("name")}/></Field><Field label="Tipe"><select className="h-10 w-full rounded-xl border bg-white px-3" {...register("type")}><option value="part">Barang/part</option><option value="service">Jasa</option><option value="other">Lainnya</option></select></Field><Field label="Satuan"><Input {...register("unit")}/></Field><Field label="Harga modal"><Input type="number" {...register("cost_price",{valueAsNumber:true})}/></Field><Field label="Harga jual"><Input type="number" {...register("selling_price",{valueAsNumber:true})}/></Field><Field label="Stok minimum"><Input type="number" step=".001" {...register("min_stock",{valueAsNumber:true})}/></Field><label className="flex items-end gap-2 pb-2 text-sm"><input type="checkbox" {...register("is_active")}/>Produk aktif</label>{mutation.error&&<p className="text-sm text-red-600 sm:col-span-2">{mutation.error.message}</p>}<div className="flex justify-end gap-2 sm:col-span-2"><Button type="button" variant="outline" onClick={()=>setOpen(false)}>Batal</Button><Button disabled={mutation.isPending}>Simpan</Button></div></form></DialogContent></Dialog></div>
}
function Field({label,error,children}:{label:string;error?:string;children:React.ReactNode}){return <label className="text-sm font-medium">{label}<div className="mt-2">{children}</div>{error&&<span className="text-xs text-red-600">{error}</span>}</label>}
