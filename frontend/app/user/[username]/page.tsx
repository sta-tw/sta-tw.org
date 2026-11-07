import type { Metadata } from "next";
import ProfileView from "./profile-view";

type PageProps = {
    params: Promise<{ username: string }>;
};

// Non-ASCII segments can arrive still percent-encoded; usernames never
// contain "%", so decoding an already-decoded value is harmless.
function decodeSegment(value: string) {
    try {
        return decodeURIComponent(value);
    } catch {
        return value;
    }
}

export async function generateMetadata({ params }: PageProps): Promise<Metadata> {
    const username = decodeSegment((await params).username);
    // The profile API needs the visitor's session cookie, so the server can't
    // look the user up here; the title only uses the URL's username.
    return {
        title: `${username} 的個人頁面 | S.T.A 特殊選才資源網`,
        description: `${username} 在 S.T.A 特殊選才資源網的個人簡介、文章與備審資料。`
    };
}

export default async function UserProfilePage({ params }: PageProps) {
    const username = decodeSegment((await params).username);
    return <ProfileView key={username} username={username} />;
}
