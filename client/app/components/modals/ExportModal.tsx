import { useState } from "react";
import { AsyncButton } from "../AsyncButton";
import SubHeader from "../primitives/SubHeader";
import FieldHelp from "../FieldHelp";

export default function ExportModal() {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const handleExport = () => {
    setLoading(true);
    fetch(`/apis/web/v1/export`, {
      method: "GET",
    })
      .then((res) => {
        if (res.ok) {
          res.blob().then((blob) => {
            const url = window.URL.createObjectURL(blob);
            const a = document.createElement("a");
            a.href = url;
            a.download = "koito_export.json";
            document.body.appendChild(a);
            a.click();
            a.remove();
            window.URL.revokeObjectURL(url);
            setLoading(false);
          });
        } else {
          res.json().then((r) => setError(r.error));
          setLoading(false);
        }
      })
      .catch((err) => {
        setError(err);
        setLoading(false);
      });
  };

  return (
    <div>
      <SubHeader>Export</SubHeader>
      <div className="flex items-center gap-1">
        <AsyncButton loading={loading} onClick={handleExport}>
          Export dated plays
        </AsyncButton>
        <FieldHelp label="Export">Exports dated plays with original aliases. Includes estimated dates selected by your filters.</FieldHelp>
      </div>
      {error && <p className="error">{error}</p>}
    </div>
  );
}
