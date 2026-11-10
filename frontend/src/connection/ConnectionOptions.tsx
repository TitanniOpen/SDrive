import { FieldHelp } from "../FieldHelp";
import type { Wizard } from "./useConnectionWizard";
import { supportsNetworkOptions } from "./methods";

export function DirectoryCacheOptions({ wizard }: { wizard: Wizard }) {
  const { t, connection: c, change, validationProps, fieldFeedback, initial, fieldID } = wizard;
  return <>
    <div className="field-with-help"><div className="check setting-switch"><input id={`${fieldID}-options-file-cache`} type="checkbox" role="switch" checked={!c.disableFileCache} disabled={initial.recovery} onChange={(e) => change({ disableFileCache: !e.target.checked })} /><span className="field-heading"><label htmlFor={`${fieldID}-options-file-cache`}>{t("Use local file cache")}</label><FieldHelp label={t("Use local file cache")}>{t("Local file cache reuses downloaded data, can speed up repeated reads and improves compatibility with applications that edit files. It uses disk space and keeps file content on this computer. Changes may wait to upload: wait for uploads to finish before shutting down, and never delete the cache while files are pending. It does not guarantee offline access. Without it, some applications may not be able to edit files. Memory buffers and directory metadata caching remain enabled.")}</FieldHelp></span></div></div>
    <div className="field-with-help">
      <div className="field-control"><div className="field-heading"><label htmlFor={`${fieldID}-options-hidden-files`}>{t("Hidden files")}</label><FieldHelp label={t("Hidden files")}>{t("Applies to names starting with a dot, including folders. Always hide excludes them from the mounted listing. Changes apply on the next connection.")}{wizard.data.platform === "linux" && <> {t("On Linux, showing dot-prefixed names depends on your file manager.")}</>}</FieldHelp></div><select id={`${fieldID}-options-hidden-files`} value={c.hiddenFileVisibility || "system"} disabled={initial.recovery} {...validationProps("hiddenFileVisibility")} aria-label={t("Hidden files")} onChange={(e) => change({ hiddenFileVisibility: e.target.value })}>
        <option value="system">{t("Follow file manager settings")}</option>
        <option value="show" disabled={wizard.data.platform === "linux"}>{t("Always show")}</option>
        <option value="hide">{t("Always hide")}</option>
      </select></div>

      {fieldFeedback("hiddenFileVisibility")}
    </div>
    <div className="field-with-help">
      <div className="field-control"><div className="field-heading"><label htmlFor={`${fieldID}-options-virtual-capacity`}>{t("Virtual capacity in Explorer (GiB)")}</label><FieldHelp label={t("Virtual capacity in Explorer (GiB)")}>{t("Zero uses the provider default. This changes the displayed total after reconnecting; it does not reserve space, enforce a quota or change provider quota or local cache free space.")}</FieldHelp></div><input id={`${fieldID}-options-virtual-capacity`} type="number" min="0" max="8388607" step="1" disabled={initial.recovery} {...validationProps("virtualCapacityGiB")} value={c.virtualCapacityGiB || 0} onChange={(e) => change({ virtualCapacityGiB: Number(e.target.value) })} /></div>
      {fieldFeedback("virtualCapacityGiB")}

    </div>
    <div className="field-with-help">
      <div className="field-control"><div className="field-heading"><label htmlFor={`${fieldID}-options-directory-lifetime`}>{t("Directory metadata lifetime (seconds)")}</label><FieldHelp label={t("Directory metadata lifetime (seconds)")}>{t("External changes appear on the next directory access after this lifetime, or after Refresh files in Explorer. Some services detect changes earlier. This is separate from SDrive status updates and applies on the next connection.")}</FieldHelp></div><input id={`${fieldID}-options-directory-lifetime`} type="number" min="1" max="86400" step="1" disabled={initial.recovery} {...validationProps("directoryCacheSeconds")} value={c.directoryCacheSeconds || 300} onChange={(e) => change({ directoryCacheSeconds: Number(e.target.value) })} /></div>
      {fieldFeedback("directoryCacheSeconds")}

    </div>
  </>;
}

export function NetworkOptions({ wizard }: { wizard: Wizard }) {
  const { t, connection: c, change, validationProps, fieldFeedback, initial, fieldID } = wizard;
  const dav = ["webdav", "nextcloud", "owncloud"].includes(c.provider);
  if (!supportsNetworkOptions(c.provider)) return null;
  return <>
    <div className="option-with-help"><span>{t("Network")}</span><FieldHelp label={t("Network")}>{t("Network changes apply on the next connection. Zero means unlimited or the engine default.")}</FieldHelp></div>
    {(["downloadLimit", "uploadLimit"] as const).map((key) => <label key={key}>
      {t(key === "downloadLimit" ? "Download limit (KiB/s)" : "Upload limit (KiB/s)")}
      <input type="number" min="0" max="1073741824" step="1" disabled={initial.recovery} {...validationProps("bandwidth")} value={(c[key] || 0) / 1024} onChange={(e) => change({ [key]: Number(e.target.value) * 1024 })} />
    </label>)}
    {fieldFeedback("bandwidth")}
    <label className="check setting-switch"><input type="checkbox" role="switch" {...validationProps("useProxy")} checked={c.useProxy || false} disabled={initial.recovery} onChange={(e) => change({ useProxy: e.target.checked })} />{t("Use the proxy configured in Preferences")}</label>
    {fieldFeedback("useProxy")}
    {dav && <>
      {(["connectTimeout", "idleTimeout"] as const).map((key) => <label key={key}>{t(key === "connectTimeout" ? "Connection timeout (seconds)" : "HTTP idle timeout (seconds)")}<input type="number" min="0" max="3600" disabled={initial.recovery} {...validationProps(key)} value={c[key] || 0} onChange={(e) => change({ [key]: Number(e.target.value) })} />{fieldFeedback(key)}</label>)}
      {wizard.auth && initial.id && <div className="field-with-help">
        <div className="field-control"><div className="field-heading"><label htmlFor={`${fieldID}-options-alternate-address`}>{t("Alternate HTTPS WebDAV address")}</label><FieldHelp label={t("Alternate HTTPS WebDAV address")}>{t("Both addresses must expose the same DAV:resource-id for the selected folder. Fallback is checked only when connecting and never replaces an active mount.")}</FieldHelp></div><input id={`${fieldID}-options-alternate-address`} type="url" {...validationProps("alternateURL")} value={c.alternateURL || ""} disabled={initial.recovery} onChange={(e) => change({ alternateURL: e.target.value })} /></div>
        {fieldFeedback("alternateURL")}

      </div>}
    </>}
  </>;
}

export function ConnectionOptions({ wizard }: { wizard: Wizard }) {
  const { t, connection: c, change, validationProps, fieldFeedback, browsers, initial, forgetCredentials, setForgetCredentials, fieldID } = wizard;
  const sessionSupported = ["webdav", "nextcloud", "owncloud", "drive", "onedrive", "sharepoint"].includes(c.provider);
  return <>
    {!initial.id && <fieldset className="option-group"><legend>{t("Files and cache")}</legend><DirectoryCacheOptions wizard={wizard} /></fieldset>}
    {!initial.id && supportsNetworkOptions(c.provider) && <fieldset className="option-group"><legend>{t("Network")}</legend><NetworkOptions wizard={wizard} /></fieldset>}
    {wizard.auth && (sessionSupported || wizard.p.oauth && c.authMethod !== "serviceAccount") && <fieldset className="option-group">
      <legend>{t("Sign-in options")}</legend>
      {wizard.p.oauth && c.authMethod !== "serviceAccount" && <>
        <label>{t("Sign-in browser")}<select {...validationProps("browser")} value={c.browser || ""} onChange={(e) => change({ browser: e.target.value })}>{browsers.map((b) => <option key={b.id} value={b.id}>{t(b.name)}</option>)}{c.browser && !browsers.some((b) => b.id === c.browser) && <option value={c.browser}>{t("Selected browser unavailable")}</option>}</select></label>
        {fieldFeedback("browser")}
        <label className="check setting-switch"><input type="checkbox" role="switch" checked={c.privateBrowser || false} onChange={(e) => change({ privateBrowser: e.target.checked })} />{t("Open a private sign-in window")}</label>
      </>}
      {sessionSupported && <>
        <div className="option-with-help">
          <div className="check setting-switch"><input id={`${fieldID}-options-session-sign-in`} type="checkbox" role="switch" checked={c.sessionOnly || false} disabled={initial.recovery} onChange={(e) => change({ sessionOnly: e.target.checked, autoMount: e.target.checked ? false : c.autoMount })} /><span className="field-heading"><label htmlFor={`${fieldID}-options-session-sign-in`}>{t("Sign in on every connection (credentials only in memory)")}</label><FieldHelp label={t("Sign in on every connection")}>{t("Session-only sign-in requires a verifiable destination. After disconnecting or reopening, sign in again to the original storage. Browser sessions may still be reused.")}</FieldHelp></span></div>

        </div>
        {fieldFeedback("sessionOnly")}
      </>}
      {c.sessionOnly && initial.id && !initial.sessionOnly && <label className="check"><input type="checkbox" required checked={forgetCredentials} onChange={(e) => setForgetCredentials(e.target.checked)} />{t("Remove this connection's saved credentials when setup is confirmed")}</label>}
    </fieldset>}
  </>;
}
