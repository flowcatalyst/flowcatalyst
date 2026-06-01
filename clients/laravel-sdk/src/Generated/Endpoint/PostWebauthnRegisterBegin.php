<?php

namespace FlowCatalyst\Generated\Endpoint;

class PostWebauthnRegisterBegin extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    /**
     * Returns a WebAuthn `PublicKeyCredentialCreationOptions` challenge. The
     * browser passes this to `navigator.credentials.create()` and posts the
     * result to `/auth/webauthn/register/complete`.
     * @param null|\FlowCatalyst\Generated\Model\RegisterBeginRequest $requestBody
     */
    public function __construct(?\FlowCatalyst\Generated\Model\RegisterBeginRequest $requestBody = null)
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
        return '/auth/webauthn/register/begin';
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        if ($this->body instanceof \FlowCatalyst\Generated\Model\RegisterBeginRequest) {
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
     * @throws \FlowCatalyst\Generated\Exception\PostWebauthnRegisterBeginBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostWebauthnRegisterBeginUnauthorizedException
     *
     * @return null|\FlowCatalyst\Generated\Model\RegisterBeginResponse
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\RegisterBeginResponse', 'json');
        }
        if (400 === $status) {
            throw new \FlowCatalyst\Generated\Exception\PostWebauthnRegisterBeginBadRequestException($response);
        }
        if (401 === $status) {
            throw new \FlowCatalyst\Generated\Exception\PostWebauthnRegisterBeginUnauthorizedException($response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}