<?php

namespace FlowCatalyst\Generated\Endpoint;

class PutFieldPolicy extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $clientId;
    /**
     * @param string $clientId A clt_ id or the client's identifier
     * @param null|\FlowCatalyst\Generated\Model\PutFieldPolicyRequest $requestBody
     */
    public function __construct(string $clientId, ?\FlowCatalyst\Generated\Model\PutFieldPolicyRequest $requestBody = null)
    {
        $this->clientId = $clientId;
        $this->body = $requestBody;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'PUT';
    }
    public function getUri(): string
    {
        return str_replace(['{clientId}'], [rawurlencode($this->clientId)], '/api/field-policies/{clientId}');
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        if ($this->body instanceof \FlowCatalyst\Generated\Model\PutFieldPolicyRequest) {
            return [['Content-Type' => ['application/json']], \FlowCatalyst\Generated\Runtime\Client\JsonPayload::encode($serializer, $this->body)];
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
     *
     * @return null|\FlowCatalyst\Generated\Model\FieldPolicyResponse|\FlowCatalyst\Generated\Model\ErrorModel
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && stripos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\FieldPolicyResponse', 'json');
        }
        if (stripos(strtolower($contentType), 'application/json') !== false) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ErrorModel', 'json');
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}