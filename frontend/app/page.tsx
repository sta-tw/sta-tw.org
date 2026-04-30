import HeroSection from "./components/hero-section";
import DataSection from "./components/data-section";
import FeatureSection from "./components/feature-section";
import JoinDiscordSection from "./components/join-discord-section";

export default function Home() {
    return (
        <main>
            <HeroSection />
            <DataSection />
            <FeatureSection />
            <JoinDiscordSection />
        </main>
    );
}

