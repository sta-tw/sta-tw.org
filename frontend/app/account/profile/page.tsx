import AccountPage from "../account-page";
import AvatarSection from "./avatar-section";
import ProfileForm from "./profile-form";

export default function Page() {
    return (
        <AccountPage title="個人資料管理" description="管理公開顯示的個人檔案與帳號資訊。">
            <AvatarSection />
            <ProfileForm />
        </AccountPage>
    );
}
