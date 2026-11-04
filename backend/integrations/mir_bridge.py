"""Presentation bridge to the user's existing deterministic parser; no MIR parsing rules.

Reads original bytes from stdin and emits passenger-linked fields with source evidence.
Never uses an LLM, substitutes financial zeroes, or treats ticket fare as selling price.
"""
import argparse
import hashlib
import json
import re
import sys
from pathlib import Path


def records_from_result(result):
    output = []
    raw_lines = [
        {"number": line.number, "text": line.data.decode("latin-1"),
         "rawHex": line.raw_hex, "terminatorHex": line.terminator_hex}
        for line in result.lines
    ]
    for booking in result.booking:
        record_number = booking["record"]
        for passenger in booking["passengers"]:
            fields = {}
            notices = [f"{issue.code}: {issue.message}" for issue in result.issues
                       if issue.record in (0, record_number)]

            def select(label, item=None):
                matches = [f for f in result.fields if f.record == record_number
                           and f.label == label and (item is None or f.item == item)
                           and f.status == "parsed" and f.raw.strip()]
                # Multiple competing values are ambiguous, never choose the first.
                return matches[0] if len(matches) == 1 else None

            def assign(key, label, item=None, money=False):
                field = select(label, item)
                if field is None:
                    return
                text = field.raw.strip(" ")
                if money and not re.fullmatch(r"-?\d+(?:\.\d+)?", text):
                    notices.append(f"{label}: unsupported financial representation retained in source.")
                    return
                fields[key] = {"value": text, "lines": [field.line], "fieldIds": [field.id]}

            item = passenger["item"]
            assign("ticketNumber", "A02TKT", item)
            # Documented carrier prefix + passenger serial, with both fields traced.
            carrier = select("T50ISA")
            serial = select("A02TKT", item)
            if carrier and serial and re.fullmatch(r"\d{3}", carrier.raw.strip()) and re.fullmatch(r"\d{10}", serial.raw.strip()):
                fields["ticketNumber"] = {
                    "value": carrier.raw.strip() + serial.raw.strip(),
                    "lines": sorted({carrier.line, serial.line}), "fieldIds": [carrier.id, serial.id],
                }
            assign("passengerName", "A02EXN", item)
            if "passengerName" not in fields:
                assign("passengerName", "A02NME", item)
            assign("pnr", "T50RCL")
            assign("airline", "T50ISN")
            assign("ticketDate", "A02DTE", item)  # Original date text; no year guessed.
            fare = passenger.get("fare")
            if fare:
                fare_item = fare["item"]
                assign("basicFare", "A07TBF", fare_item, money=True)
                assign("mirTotalFare", "A07TTA", fare_item, money=True)
                assign("basicCurrency", "A07CRB", fare_item)
                assign("fareCurrency", "A07CRT", fare_item)
            # Only passenger-linked flight items. Never assign the entire booking's
            # itinerary or a whole-party fare to an individual passenger.
            linked = {basis["fields"].get("A08ITN") for basis in passenger.get("fare_basis", [])}
            segments = [segment for segment in booking["sections"].get("A04", [])
                        if segment["fields"].get("A04ITN") in linked]
            for key, labels in [("sector", ("A04OCC", "A04DCC")), ("travelDate", ("A04DTL",))]:
                collected = []
                evidence = []
                for segment in segments:
                    values = [select(label, segment["item"]) for label in labels]
                    if all(values):
                        collected.append(" → ".join(f.raw.strip() for f in values))
                        evidence.extend(values)
                if collected:
                    fields[key] = {"value": " / ".join(collected),
                                   "lines": sorted({f.line for f in evidence}),
                                   "fieldIds": [f.id for f in evidence]}
            # Missing client/supplier amounts, GST and commission remain missing.
            if "ticketNumber" not in fields:
                notices.append("Ticket number is missing or ambiguous; verify before sale creation.")
            output.append({
                "id": f"{result.sha256}:r{record_number}:p{item}", "fileName": result.filename,
                "sha256": result.sha256, "recordDate": "",
                "status": "Failed" if any(issue.severity == "error" for issue in result.issues)
                          else "Validation required" if notices else "Extracted",
                "fields": fields, "rawLines": raw_lines, "issues": notices,
            })
    if not output:
        output.append({"id": f"{result.sha256}:unresolved", "fileName": result.filename,
                       "sha256": result.sha256, "recordDate": "", "status": "Failed", "fields": {},
                       "rawLines": raw_lines,
                       "issues": ["No supported passenger-linked ticket record found."] +
                                 [f"{issue.code}: {issue.message}" for issue in result.issues]})
    return output


def main():
    args = argparse.ArgumentParser()
    args.add_argument("--parser-root", required=True)
    args.add_argument("--filename", required=True)
    options = args.parse_args()
    root = Path(options.parser_root).resolve(strict=True)
    if not (root / "parser.py").is_file():
        raise RuntimeError("Existing parser.py is not available")
    sys.path.insert(0, str(root))
    from parser import parse_mir
    data = sys.stdin.buffer.read((2 << 20) + 1)
    if len(data) > 2 << 20:
        raise ValueError("MIR file exceeds 2 MB")
    result = parse_mir(data, options.filename)
    if result.sha256 != hashlib.sha256(data).hexdigest():
        raise ValueError("Parser source hash mismatch")
    json.dump(records_from_result(result), sys.stdout, ensure_ascii=True)


if __name__ == "__main__":
    main()
