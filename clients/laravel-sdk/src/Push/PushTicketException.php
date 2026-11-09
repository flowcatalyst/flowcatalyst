<?php

declare(strict_types=1);

namespace FlowCatalyst\Push;

use FlowCatalyst\Exceptions\FlowCatalystException;

/**
 * A ticket (or ticket key) the SDK refuses to build: the gateway would grant
 * nothing for it.
 */
final class PushTicketException extends FlowCatalystException
{
}
