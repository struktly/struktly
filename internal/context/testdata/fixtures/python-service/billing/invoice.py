"""Invoice totals and rounding."""


def invoice_total(lines, tax_rate):
    """Return the rounded total for one invoice."""
    subtotal = sum(line.amount for line in lines)
    return round(subtotal * (1 + tax_rate), 2)
