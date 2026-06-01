<?php

namespace FlowCatalyst\Generated\Endpoint;

class PostWebauthnAuthenticateComplete extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    /**
     * Validates the assertion, applies counter / backup-state updates,
     * re-checks the federation gate (hard cutover), and on success issues a
     * session cookie. All failure modes return 401 `INVALID_CREDENTIALS` with
     * an identical shape to defeat enumeration.
     * @param null|\FlowCatalyst\Generated\Model\AuthenticateCompleteRequest $requestBody
     */
    public function __construct(?\FlowCatalyst\Generated\Model\AuthenticateCompleteRequest $requestBody = null)
    {
        $this->body = $requestBody;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'POST';
    }
    public function getUri(): string
    {
        return '/auth/webauthn/authenticate/complete';
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        if ($this->body instanceof \FlowCatalyst\Generated\Model\AuthenticateCompleteRequest) {
            return [['Content-Type' => ['application/json']], $serializer->serialize($this->body, 'json')];
        }
        return [[], null];
    }
    public function getExtraHeaders(): array
    {
        return ['Accept' => ['application/json']];
    }
    /**
     * {@inheritdoc}
     *
     * @throws \FlowCatalyst\Generated\Exception\PostWebauthnAuthenticateCompleteUnauthorizedException
     *
     * @return null|\FlowCatalyst\Generated\Model\AuthenticateCompleteResponse
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\AuthenticateCompleteResponse', 'json');
        }
        if (401 === $status) {
            throw new \FlowCatalyst\Generated\Exception\PostWebauthnAuthenticateCompleteUnauthorizedException($response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}