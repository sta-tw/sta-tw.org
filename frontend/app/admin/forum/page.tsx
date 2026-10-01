import type { Metadata } from "next";
import ForumModerationView from "./forum-moderation-view";

export const metadata: Metadata = {
    title: "論壇管理 | S.T.A 管理後台",
    robots: { index: false, follow: false }
};

export default function AdminForumPage() {
    return <ForumModerationView />;
}
