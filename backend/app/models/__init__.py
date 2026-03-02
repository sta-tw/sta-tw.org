# Import all models so Alembic can discover them
from app.models.user import User, UserSession  # noqa: F401
from app.models.channel import Channel  # noqa: F401
from app.models.message import Message, MessageReaction  # noqa: F401
from app.models.audit import AuditLog  # noqa: F401
from app.models.verification import VerificationRequest  # noqa: F401
from app.models.ticket import Ticket, TicketMessage  # noqa: F401
from app.models.admission import AdmissionDocument  # noqa: F401
from app.models.portfolio import PortfolioDocument  # noqa: F401
from app.models.portfolio_scoring import PortfolioScoringRule, PortfolioViewLog  # noqa: F401
from app.models.portfolio_school import SchoolOption, SchoolRequest  # noqa: F401
