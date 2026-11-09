import { useId, useState } from "react";
import { createPortal } from "react-dom";
import { api, type ConnectionView } from "./api";
import { useLanguage } from "./i18n";
import { useDialog } from "./useDialog";
import { DialogLayout } from "./DialogLayout";
import { errorMessage } from "./messages";

function Confirmation({ connection, close, reset }: {
  connection: ConnectionView;
  close: () => void;
  reset: boolean;
}) {
  const { t } = useLanguage();
  const id = useId();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>();
  const label = reset ? "Start with a fresh cache" : "Preserve and disconnect";
  const cancel = () => { if (!busy) close(); };
  const ref = useDialog(cancel);
  const confirm = async () => {
    setBusy(true);
    setError(undefined);
    try {
      if (reset) await api.resolveCacheRecovery(connection.id);
      else await api.preserveAndDisconnect(connection.id);
      close();
    } catch (cause) {
      setError(cause);
    } finally {
      setBusy(false);
    }
  };
  return <DialogLayout centered dialogRef={ref} title={t(label)} titleID={id}
    onClose={cancel} closeDisabled={busy} footer={<>
      <button disabled={busy} onClick={cancel}>{t("Cancel")}</button>
      <button disabled={busy} onClick={() => void confirm()}>{t(busy ? "Working…" : label)}</button>
    </>}>
    <p>{t(reset
      ? "Confirm that you reviewed the preserved files. The active cache will be reset; preserved copies will remain. No files will be uploaded automatically."
      : "Files will be preserved on this computer. This does not confirm that they were uploaded. Close open files before continuing.")}</p>
    <p>{t("Preserved cache files may contain partial content. Review their metadata before restoring.")}</p>
    {connection.cache.recoveryPath && <>
      <p>{t("Local recovery is pending. Preserved files:")} {connection.cache.recoveryPath}</p>
      <button type="button" disabled={busy} onClick={() => {
        void api.open(connection.id).catch(setError);
      }}>{t("Open preserved files")}</button>
    </>}
    {error !== undefined && <p role="alert">{t(errorMessage(error))}</p>}
  </DialogLayout>;
}

export function CachePreservation({ connection, disabled = false }: {
  connection: ConnectionView;
  disabled?: boolean;
}) {
  const { t } = useLanguage();
  const [action, setAction] = useState<"preserve" | "reset">();
  return <>
    <button type="button" disabled={disabled} onClick={() => setAction("preserve")}>
      {t("Preserve and disconnect")}
    </button>
    {connection.cache.recoveryPath && !connection.mounted &&
      <button type="button" disabled={disabled} onClick={() => setAction("reset")}>
        {t("Start with a fresh cache")}
      </button>}
    {action && createPortal(<Confirmation reset={action === "reset"} connection={connection}
      close={() => setAction(undefined)} />, document.body)}
  </>;
}
