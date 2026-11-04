import type { Metadata } from "next";
import SavedView from "./saved-view";

export const metadata: Metadata = {
    title: "已儲存的項目 | S.T.A 特殊選才資源網"
};

export default function Page() {
    return <SavedView />;
}
