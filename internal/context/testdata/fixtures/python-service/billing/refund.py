"""Refunds against a settled invoice."""


def refund_amount(invoice, reason):
    """Return the refundable amount for a settled invoice."""
    if reason == "duplicate":
        return invoice.total
    return 0.0
