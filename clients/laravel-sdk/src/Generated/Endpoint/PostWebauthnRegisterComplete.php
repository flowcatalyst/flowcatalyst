<?php

namespace FlowCatalyst\Generated\Endpoint;

class PostWebauthnRegisterComplete extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    /**
     * Validates the browser's attestation response and stores the credential.
     * @param null|\FlowCatalyst\Generated\Model\RegisterCompleteRequest $requestBody
     */
    public function __construct(?\FlowCatalyst\Generated\Model\RegisterCompleteRequest $requestBody = null)
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
        return '/auth/webauthn/register/complete';
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        if ($this->body instanceof \FlowCatalyst\Generated\Model\RegisterCompleteRequest) {
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
     * @throws \FlowCatalyst\Generated\Exception\PostWebauthnRegisterCompleteBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostWebauthnRegisterCompleteUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\PostWebauthnRegisterCompleteForbiddenException
     *
     * @return null|\FlowCatalyst\Generated\Model\RegisterCompleteResponse
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\RegisterCompleteResponse', 'json');
        }
        if (400 === $status) {
            throw new \FlowCatalyst\Generated\Exception\PostWebauthnRegisterCompleteBadRequestException($response);
        }
        if (401 === $status) {
            throw new \FlowCatalyst\Generated\Exception\PostWebauthnRegisterCompleteUnauthorizedException($response);
        }
        if (403 === $status) {
            throw new \FlowCatalyst\Generated\Exception\PostWebauthnRegisterCompleteForbiddenException($response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}