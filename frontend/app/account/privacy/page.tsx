import AccountPage from "../account-page";
import PrivacyForm from "./privacy-form";

export default function Page() {
    return (
        <AccountPage
            title="隱私權設定"
            description="決定誰能看到你的個人檔案，以及要公開哪些內容。"
        >
            <PrivacyForm />
        </AccountPage>
    );
}
