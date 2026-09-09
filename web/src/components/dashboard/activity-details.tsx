"use client";

import { CopyIcon, FileTextIcon } from "lucide-react";
import { toast } from "sonner";

import type { ActivityEvent } from "@/components/dashboard/types";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";

const kinds: Record<string, { label: string; explanation: string }> = {
  dns_timeout: {
    label: "Timeout de DNS",
    explanation: "A resolução de um endereço ultrapassou o prazo. Isso não comprova que o site esteja fora do ar.",
  },
  timeout: {
    label: "Tempo limite excedido",
    explanation: "Uma etapa não terminou dentro do prazo. A mensagem abaixo ajuda a identificar se foi navegação, captura ou comunicação com o worker.",
  },
  canceled: {
    label: "Chamada cancelada",
    explanation: "A execução foi cancelada. Pode acontecer por interrupção do cliente ou encerramento da conexão; este registro não identifica sozinho a origem.",
  },
  transport: {
    label: "Falha de comunicação",
    explanation: "O control plane não recebeu uma resposta completa do worker. Verifique disponibilidade, rede e possíveis reinicializações no horário do evento.",
  },
  tool_error: {
    label: "Erro da operação",
    explanation: "A operação retornou um erro. Consulte a mensagem registrada antes de repetir a ação.",
  },
};

const stages: Record<string, string> = {
  selection: "Seleção do navegador / parâmetros",
  worker_endpoint: "Disponibilidade do worker",
  transport: "Comunicação control plane → worker",
  worker: "Execução da ferramenta no worker",
  human_access: "Criação do acesso humano",
  saved_login: "Login com acesso salvo",
};

export function ActivityDetails({ event }: { event: ActivityEvent }) {
  const details = event.diagnostics;
  const failure = event.result !== "success";
  const category = details?.kind ? kinds[details.kind] : undefined;
  const duration = details?.duration_ms;
  const time = new Date(event.created_at);
  const timestamp = Number.isNaN(time.getTime())
    ? event.created_at
    : time.toLocaleString("pt-BR", { timeZoneName: "short" });
  const log = JSON.stringify({
    id: event.id,
    event: event.event,
    result: event.result,
    browser_id: event.browser_id,
    browser_name: event.browser_name,
    created_at: event.created_at,
    ...(details ?? {}),
  }, null, 2);

  async function copyLog() {
    try {
      await navigator.clipboard.writeText(log);
      toast.success("Detalhes copiados.");
    } catch {
      toast.error("Não foi possível copiar. Selecione o texto da mensagem para copiar manualmente.");
    }
  }

  return (
    <Dialog>
      <DialogTrigger render={<Button variant="ghost" size="sm" />} aria-label={`Ver detalhes de ${event.event} em ${timestamp}`}>
        <FileTextIcon data-icon="inline-start" />
        Ver detalhes
      </DialogTrigger>
      <DialogContent className="max-h-[85dvh] overflow-y-auto sm:max-w-xl" showCloseButton={false}>
        <DialogHeader>
          <DialogTitle>{failure ? "Detalhes da falha" : "Detalhes da atividade"}</DialogTitle>
          <DialogDescription>
            {category?.explanation ?? "Registro desta operação no Navego. A duração, quando disponível, é medida pelo control plane e não inclui o tempo de raciocínio do ChatGPT."}
          </DialogDescription>
        </DialogHeader>
        <dl className="grid min-w-0 grid-cols-1 gap-4 rounded-lg border p-4 sm:grid-cols-2">
          <Detail label="Operação" value={event.event} />
          <Detail label="Navegador" value={event.browser_name || event.browser_id || "Não associado / removido"} />
          <Detail label="Horário" value={timestamp} />
          <Detail label="Duração no control plane" value={duration === undefined ? "Não registrada" : `${(duration / 1000).toLocaleString("pt-BR", { maximumFractionDigits: 3 })} s`} />
          <Detail label="Tipo" value={category?.label ?? (failure ? "Não identificado" : "Concluído")} />
          <Detail label="Etapa" value={details?.stage ? (stages[details.stage] ?? details.stage) : "Não registrada"} />
          {details?.error_code ? <Detail label="Código retornado pela ferramenta" value={details.error_code} /> : null}
          <Detail label="ID do evento" value={event.id} />
        </dl>
        <div className="flex min-w-0 flex-col gap-2">
          <h3 className="text-sm font-medium">Mensagem registrada</h3>
          {details?.error ? (
            <pre tabIndex={0} className="max-h-64 overflow-auto whitespace-pre-wrap break-words rounded-lg border bg-muted/40 p-4 font-mono text-xs leading-6">
              {details.error}
            </pre>
          ) : (
            <p className="text-sm leading-6 text-muted-foreground">
              {failure
                ? "Este evento não possui mensagem de erro salva. Registros antigos podem conter apenas o status; não é possível recuperar retroativamente um detalhe que não foi registrado."
                : "Operação concluída sem erro registrado."}
            </p>
          )}
          <p className="text-xs leading-5 text-muted-foreground">
            Diagnóstico da chamada, não o log completo do container. Argumentos e conteúdo das páginas não são incluídos; URLs e padrões de credenciais são ocultados na mensagem.
          </p>
        </div>
        <DialogFooter>
          <DialogClose render={<Button variant="outline" />}>Fechar</DialogClose>
          <Button type="button" onClick={copyLog}>
            <CopyIcon data-icon="inline-start" />
            Copiar detalhes
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function Detail({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex min-w-0 flex-col gap-1">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="break-words text-sm">{value}</dd>
    </div>
  );
}
